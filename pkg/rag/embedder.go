package rag

import (
	"context"
	"fmt"

	"github.com/sashabaranov/go-openai"
)

// Embedder 文本向量化接口
// 将自然语言文本转换为固定维度的浮点向量，用于向量相似度检索
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	// Dim 返回向量维度，创建 Milvus Collection 时需要
	Dim() int
}

// KnownModelDimensions 主流 Embedding 模型的默认维度映射表
// 所有列出的模型均兼容 OpenAI /v1/embeddings 协议（含国内厂商 OpenAI 兼容接口）
var KnownModelDimensions = map[string]int{
	// OpenAI
	"text-embedding-3-small": 1536,
	"text-embedding-3-large": 3072,
	"text-embedding-ada-002": 1536,
	// BGE 系列（通过 xinference / Ollama / SiliconFlow 部署）
	"bge-m3":             1024,
	"bge-large-zh-v1.5": 1024,
	"bge-base-zh-v1.5":  768,
	// 火山引擎 / 豆包
	"doubao-embedding":       2048,
	"doubao-embedding-large": 4096,
	// 通义千问
	"text-embedding-v3": 1024,
}

// OpenAIEmbedder 兼容 OpenAI /v1/embeddings 协议的通用 Embedder 实现
// 支持 OpenAI、豆包、BGE-M3（via xinference/Ollama/SiliconFlow）等任意兼容厂商
type OpenAIEmbedder struct {
	client *openai.Client
	model  string
	dim    int
}

// NewEmbedder 通用工厂函数（推荐使用）
// model: 模型名称，如 "text-embedding-3-small"、"bge-m3"；空字符串则使用 text-embedding-3-small
// dim:   向量维度；传 0 时从 KnownModelDimensions 自动查找，若未知则默认 1536
func NewEmbedder(apiKey, baseURL, model string, dim int) Embedder {
	if model == "" {
		model = string(openai.SmallEmbedding3)
	}
	if dim <= 0 {
		if d, ok := KnownModelDimensions[model]; ok {
			dim = d
		} else {
			dim = 1536
		}
	}
	cfg := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		cfg.BaseURL = baseURL
	}
	return &OpenAIEmbedder{
		client: openai.NewClientWithConfig(cfg),
		model:  model,
		dim:    dim,
	}
}

// NewOpenAIEmbedder 保留向后兼容，内部委托给 NewEmbedder
func NewOpenAIEmbedder(apiKey, baseURL string) *OpenAIEmbedder {
	return NewEmbedder(apiKey, baseURL, "", 0).(*OpenAIEmbedder)
}

func (e *OpenAIEmbedder) Dim() int { return e.dim }

// Embed 将文本转换为向量
func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	resp, err := e.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Input: []string{text},
		Model: openai.EmbeddingModel(e.model),
	})
	if err != nil {
		return nil, fmt.Errorf("embedding 请求失败: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("embedding 响应为空")
	}
	return resp.Data[0].Embedding, nil
}
