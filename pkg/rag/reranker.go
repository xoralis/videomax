package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Reranker 二次排序接口
// 接收原始 query 和候选文档列表，返回按相关性降序排列的文档列表
type Reranker interface {
	Rerank(ctx context.Context, query string, docs []Document) ([]Document, error)
}

// ── HTTP Reranker（兼容 SiliconFlow / BGE-Reranker / Jina 等 /v1/rerank API）────

// rerankRequest /v1/rerank 请求体（兼容 SiliconFlow、Cohere、Jina 格式）
type rerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n,omitempty"`
}

// rerankResult 单条重排结果
type rerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

// rerankResponse /v1/rerank 响应体
type rerankResponse struct {
	Results []rerankResult `json:"results"`
}

// HTTPReranker 调用外部 /v1/rerank HTTP API 的重排器实现
type HTTPReranker struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewHTTPReranker 创建 HTTPReranker
// baseURL: 如 "https://api.siliconflow.cn"
// model:   如 "BAAI/bge-reranker-v2-m3"
func NewHTTPReranker(baseURL, apiKey, model string) *HTTPReranker {
	return &HTTPReranker{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Rerank 调用 /v1/rerank API，按相关性降序重排文档并返回
func (r *HTTPReranker) Rerank(ctx context.Context, query string, docs []Document) ([]Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	ctx, span := otel.Tracer("videomax").Start(ctx, "rag.reranker.rerank",
		trace.WithAttributes(
			attribute.String("gen_ai.operation.name", "reranker"),
			attribute.String("gen_ai.request.model", r.model),
			attribute.String("gen_ai.prompt", query),
			attribute.Int("rag.candidate_count", len(docs)),
		))
	defer span.End()

	contents := make([]string, len(docs))
	for i, doc := range docs {
		contents[i] = doc.Content
	}

	reqBody := rerankRequest{
		Model:     r.model,
		Query:     query,
		Documents: contents,
		TopN:      len(docs),
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("rerank 请求序列化失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/v1/rerank", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("构建 rerank 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if r.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+r.apiKey)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("rerank API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("rerank API 返回非 200 状态码: %d", resp.StatusCode)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	var result rerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("解析 rerank 响应失败: %w", err)
	}

	// 按 relevance_score 降序排列
	sort.Slice(result.Results, func(i, j int) bool {
		return result.Results[i].RelevanceScore > result.Results[j].RelevanceScore
	})

	reranked := make([]Document, 0, len(result.Results))
	scoreLines := make([]string, 0, len(result.Results))
	for _, item := range result.Results {
		if item.Index >= 0 && item.Index < len(docs) {
			reranked = append(reranked, docs[item.Index])
			scoreLines = append(scoreLines, fmt.Sprintf("[%d] score=%.4f id=%s content:\n%s\n",
				item.Index, item.RelevanceScore, docs[item.Index].ID, docs[item.Index].Content))
		}
	}
	span.SetAttributes(
		attribute.Int("rag.result_count", len(reranked)),
		attribute.String("gen_ai.completion", strings.Join(scoreLines, "\n")),
	)
	return reranked, nil
}
