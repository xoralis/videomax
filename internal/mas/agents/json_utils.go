package agents

import (
	"encoding/json"
	"regexp"
	"strings"
)

// extractJSONBlock 从 LLM 响应中提取 JSON 字符串。
// LLM 常以 ```json ... ``` 代码块包裹输出，此函数同时兼容裸 JSON 和代码块两种形式。
func extractJSONBlock(text string) string {
	// 优先匹配 ```json ... ``` 代码块
	re := regexp.MustCompile("(?s)```(?:json)?\\s*([\\[{].*?[\\]}])\\s*```")
	if m := re.FindStringSubmatch(text); len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	// 退回：提取第一个 { 到最后一个 } 之间的内容
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start != -1 && end > start {
		return text[start : end+1]
	}
	// 尝试数组
	start = strings.Index(text, "[")
	end = strings.LastIndex(text, "]")
	if start != -1 && end > start {
		return text[start : end+1]
	}
	return text
}

// parseStringField 从 JSON 对象中提取指定字段的字符串值。
// 若解析失败或字段不存在，则返回 fallback 值（通常为原始文本）。
func parseStringField(jsonText, field, fallback string) string {
	raw := extractJSONBlock(jsonText)
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return fallback
	}
	v, ok := obj[field]
	if !ok {
		return fallback
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return fallback
	}
	return s
}

// parseBoolField 从 JSON 对象中提取指定字段的布尔值。
// 若解析失败则返回 fallback。
func parseBoolField(jsonText, field string, fallback bool) bool {
	raw := extractJSONBlock(jsonText)
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return fallback
	}
	v, ok := obj[field]
	if !ok {
		return fallback
	}
	var b bool
	if err := json.Unmarshal(v, &b); err != nil {
		return fallback
	}
	return b
}
