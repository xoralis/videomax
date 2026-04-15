package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"video-max/pkg/rag"
)

// RAGSearchTool 基于向量检索的最佳实践查询工具
// 替代 PresetSearchTool 的硬编码规则，改为从 Milvus 知识库中语义检索
// 完整实现 AITool 接口，可直接注册给 VisualAgent 的 ReAct 循环
type RAGSearchTool struct {
	retriever *rag.Retriever
}

// NewRAGSearchTool 创建 RAGSearchTool 实例
func NewRAGSearchTool(retriever *rag.Retriever) *RAGSearchTool {
	return &RAGSearchTool{retriever: retriever}
}

// Retriever 返回内部 Retriever 实例，供 Kafka Consumer 写回历史 Prompt 使用
func (t *RAGSearchTool) Retriever() *rag.Retriever {
	return t.retriever
}

// ragSearchParams 工具的入参结构，与 ParametersSchema 一一对应
type ragSearchParams struct {
	Query string `json:"query"` // 需要查询的自然语言问题
}

func (t *RAGSearchTool) Name() string {
	return "search_best_practices"
}

func (t *RAGSearchTool) Description() string {
	return "从知识库中检索视频生成的最佳实践规则，包括各供应商推荐的分辨率、时长、风格关键词、运镜指令和提示词写作技巧。当你需要了解某个平台的参数规范或提示词写法时，调用此工具。"
}

func (t *RAGSearchTool) ParametersSchema() string {
	return `{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "自然语言查询，例如：'bytedance 推荐的分辨率和时长' 或 'kling 运镜关键词'"
			}
		},
		"required": ["query"]
	}`
}

// Execute 接收大模型生成的 JSON 参数，执行向量检索并返回 Observation 文本
func (t *RAGSearchTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	ctx, span := otel.Tracer("videomax").Start(ctx, "rag_search_best_practices",
		trace.WithAttributes(
			attribute.String("gen_ai.operation.name", "tool"),
			attribute.String("gen_ai.tool.name", "search_best_practices"),
			attribute.String("gen_ai.prompt", argsJSON),
		))
	defer span.End()

	var params ragSearchParams
	if err := json.Unmarshal([]byte(argsJSON), &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	if params.Query == "" {
		return "", fmt.Errorf("query 不能为空")
	}

	docs, err := t.retriever.Retrieve(ctx, params.Query)
	if err != nil {
		return "", fmt.Errorf("知识库检索失败: %w", err)
	}

	result := rag.FormatResults(docs)
	span.SetAttributes(attribute.String("gen_ai.completion", result))
	return result, nil
}
