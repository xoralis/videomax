package rag

import (
	"context"
	"fmt"
	"strings"
)

// Retriever 将 Embedder 和 VectorStore 组合为「检索器」
// 对外暴露一个统一的 Retrieve(query string) -> []Document 接口
// 内部自动完成：文本 → 向量 → 相似度检索 三步流程
type Retriever struct {
	embedder Embedder
	store    VectorStore
	topK     int
}

// NewRetriever 创建检索器，topK 指定每次检索返回的最大文档数
func NewRetriever(embedder Embedder, store VectorStore, topK int) *Retriever {
	return &Retriever{
		embedder: embedder,
		store:    store,
		topK:     topK,
	}
}

// WithTopK 返回一个使用新 topK 值的浅拷贝 Retriever（原实例不变）
func (r *Retriever) WithTopK(topK int) *Retriever {
	clone := *r
	clone.topK = topK
	return &clone
}

// Retrieve 对 query 文本进行语义检索，返回最相关的文档列表
func (r *Retriever) Retrieve(ctx context.Context, query string) ([]Document, error) {
	vec, err := r.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query 向量化失败: %w", err)
	}
	docs, err := r.store.Search(ctx, vec, r.topK)
	if err != nil {
		return nil, fmt.Errorf("向量检索失败: %w", err)
	}
	return docs, nil
}

// IngestDocuments 批量向量化并写入知识库（入库工具调用此方法）
func (r *Retriever) IngestDocuments(ctx context.Context, docs []Document) error {
	for i := range docs {
		vec, err := r.embedder.Embed(ctx, docs[i].Content)
		if err != nil {
			return fmt.Errorf("文档 '%s' 向量化失败: %w", docs[i].ID, err)
		}
		docs[i].Embedding = vec
	}
	return r.store.Upsert(ctx, docs)
}

// FormatResults 将检索结果格式化为大模型可直接阅读的字符串（Observation）
func FormatResults(docs []Document) string {
	if len(docs) == 0 {
		return "未找到相关知识库内容。"
	}
	var sb strings.Builder
	for i, doc := range docs {
		sb.WriteString(fmt.Sprintf("[%d] %s\n", i+1, doc.Content))
	}
	return strings.TrimSpace(sb.String())
}
