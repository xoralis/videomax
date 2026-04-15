package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/net/html"
)

// DuckDuckGoTool 通过 DuckDuckGo Lite 搜索互联网实时信息
// 通过解析 DuckDuckGo HTML Lite 页面获取搜索结果，无需 API Key
type DuckDuckGoTool struct {
	httpClient *http.Client
	maxResults int
}

// NewDuckDuckGoTool 创建一个默认返回 5 条结果的搜索工具
func NewDuckDuckGoTool() *DuckDuckGoTool {
	return &DuckDuckGoTool{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		maxResults: 5,
	}
}

type ddgSearchParams struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results,omitempty"`
}

// ddgResult 单条搜索结果
type ddgResult struct {
	Title   string
	URL     string
	Snippet string
}

func (t *DuckDuckGoTool) Name() string { return "web_search" }

func (t *DuckDuckGoTool) Description() string {
	return "使用 DuckDuckGo 搜索互联网上的实时信息，适合查询最新资讯、技术文档、产品规格等知识库中不存在的内容。返回搜索结果标题、链接和摘要。"
}

func (t *DuckDuckGoTool) ParametersSchema() string {
	return `{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "搜索关键词或自然语言查询，例如：'kling ai video generation api parameters 2024'"
			},
			"max_results": {
				"type": "integer",
				"description": "返回结果条数，默认 5，最多 10",
				"minimum": 1,
				"maximum": 10
			}
		},
		"required": ["query"]
	}`
}

// Execute 调用 DuckDuckGo Lite 进行搜索并返回格式化结果
func (t *DuckDuckGoTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	_, span := otel.Tracer("videomax").Start(ctx, "web_search",
		trace.WithAttributes(
			attribute.String("gen_ai.operation.name", "tool"),
			attribute.String("gen_ai.tool.name", "web_search"),
			attribute.String("gen_ai.prompt", argsJSON),
		))
	defer span.End()

	var params ddgSearchParams
	if err := json.Unmarshal([]byte(argsJSON), &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	if params.Query == "" {
		return "", fmt.Errorf("query 不能为空")
	}

	limit := t.maxResults
	if params.MaxResults > 0 && params.MaxResults <= 10 {
		limit = params.MaxResults
	}

	results, err := t.search(ctx, params.Query, limit)
	if err != nil {
		return "", fmt.Errorf("DuckDuckGo 搜索失败: %w", err)
	}

	if len(results) == 0 {
		return "未找到相关搜索结果。", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("DuckDuckGo 搜索「%s」共返回 %d 条结果：\n\n", params.Query, len(results)))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("[%d] %s\n", i+1, r.Title))
		sb.WriteString(fmt.Sprintf("    URL: %s\n", r.URL))
		if r.Snippet != "" {
			sb.WriteString(fmt.Sprintf("    摘要: %s\n", r.Snippet))
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// search 向 DuckDuckGo Lite 发起请求并解析 HTML 结果
func (t *DuckDuckGoTool) search(ctx context.Context, query string, limit int) ([]ddgResult, error) {
	searchURL := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query) + "&kl=cn-zh"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}
	// 模拟浏览器 User-Agent，避免被直接拒绝
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024)) // 最多读 2 MB
	if err != nil {
		return nil, err
	}

	return parseResults(string(body), limit), nil
}

// parseResults 从 DuckDuckGo Lite HTML 中提取搜索结果
func parseResults(body string, limit int) []ddgResult {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return nil
	}

	var results []ddgResult
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(results) >= limit {
			return
		}
		if isResultDiv(n) {
			if r, ok := extractResult(n); ok {
				results = append(results, r)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return results
}

// isResultDiv 判断节点是否为搜索结果容器（class 含 "result"，但排除广告）
func isResultDiv(n *html.Node) bool {
	if n.Type != html.ElementNode || n.Data != "div" {
		return false
	}
	for _, a := range n.Attr {
		if a.Key == "class" {
			cls := a.Val
			return strings.Contains(cls, "result") &&
				strings.Contains(cls, "web-result") &&
				!strings.Contains(cls, "result--ad")
		}
	}
	return false
}

// extractResult 从结果容器节点中提取标题、URL 和摘要
func extractResult(n *html.Node) (ddgResult, bool) {
	var r ddgResult
	var extract func(*html.Node)
	extract = func(node *html.Node) {
		if node.Type == html.ElementNode {
			cls := attrVal(node, "class")
			switch {
			case node.Data == "a" && strings.Contains(cls, "result__a"):
				r.Title = textContent(node)
				href := attrVal(node, "href")
				if href != "" {
					r.URL = resolveURL(href)
				}
			case strings.Contains(cls, "result__snippet"):
				r.Snippet = strings.TrimSpace(textContent(node))
			case node.Data == "span" && strings.Contains(cls, "result__url"):
				if r.URL == "" {
					r.URL = strings.TrimSpace(textContent(node))
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}
	}
	extract(n)

	if r.Title == "" || r.URL == "" {
		return r, false
	}
	return r, true
}

// resolveURL 解析 DuckDuckGo 重定向链接，提取真实 URL
// DuckDuckGo Lite 的链接格式为 //duckduckgo.com/l/?uddg=https%3A%2F%2F...
func resolveURL(href string) string {
	if strings.HasPrefix(href, "//") {
		href = "https:" + href
	}
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	// 提取 uddg 参数中的真实 URL
	if uddg := u.Query().Get("uddg"); uddg != "" {
		if decoded, err := url.QueryUnescape(uddg); err == nil {
			return decoded
		}
	}
	return href
}

// attrVal 获取节点指定属性的值
func attrVal(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// textContent 递归提取节点的纯文本内容
func textContent(n *html.Node) string {
	var sb strings.Builder
	var collect func(*html.Node)
	collect = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			collect(c)
		}
	}
	collect(n)
	return strings.TrimSpace(sb.String())
}
