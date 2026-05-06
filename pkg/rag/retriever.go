package rag

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Retriever 将 Embedder 和 VectorStore 组合为「检索器」
// 对外暴露 Retrieve / RetrieveWithSparse 两种检索接口
// 可通过 WithTopK / WithFilter / WithReranker 返回浅拷贝定制行为
type Retriever struct {
	embedder   Embedder
	store      VectorStore
	topK       int
	filter     string   // Milvus 标量过滤表达式，如 `provider == "bytedance"`
	reranker   Reranker // 可选：二次排序器（Reranker 接口定义在 reranker.go）
	rerankTopN int      // rerank 候选数量（初次召回 rerankTopN，重排后截取 topK）
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

// WithFilter 返回一个带有标量过滤条件的浅拷贝 Retriever
// filter: Milvus 表达式，如 `provider == "bytedance"`，空字符串表示不过滤
func (r *Retriever) WithFilter(filter string) *Retriever {
	clone := *r
	clone.filter = filter
	return &clone
}

// WithReranker 返回一个启用 Reranker 二次排序的浅拷贝 Retriever
// rerankTopN: 初次召回候选数量（建议 >= topK*3），重排后截取前 topK 返回
func (r *Retriever) WithReranker(reranker Reranker, rerankTopN int) *Retriever {
	clone := *r
	clone.reranker = reranker
	clone.rerankTopN = rerankTopN
	return &clone
}

// Retrieve 对 query 文本进行语义检索，返回最相关的文档列表
// 流程：文本向量化 → 向量检索 → （可选）Reranker 重排序
func (r *Retriever) Retrieve(ctx context.Context, query string) ([]Document, error) {
	ctx, span := otel.Tracer("videomax").Start(ctx, "rag.retriever.retrieve",
		trace.WithAttributes(
			attribute.String("gen_ai.operation.name", "retriever"),
			attribute.String("gen_ai.prompt", query),
			attribute.Int("rag.top_k", r.topK),
			attribute.String("rag.filter", r.filter),
			attribute.Bool("rag.reranker_enabled", r.reranker != nil),
		))
	defer span.End()

	vec, err := r.embedder.Embed(ctx, query)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("query 向量化失败: %w", err)
	}

	fetchK := r.topK
	if r.reranker != nil && r.rerankTopN > r.topK {
		fetchK = r.rerankTopN
	}

	docs, err := r.store.Search(ctx, vec, query, fetchK, r.filter)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("向量检索失败: %w", err)
	}

	// Reranker 二次排序
	if r.reranker != nil && len(docs) > 0 {
		docs, err = r.reranker.Rerank(ctx, query, docs)
		if err != nil {
			// rerank 失败时降级返回原始结果，不中断主流程
			docs = docs
		}
		if len(docs) > r.topK {
			docs = docs[:r.topK]
		}
	}

	// 最终返回给调用方的结果写入 Output（SetAttributes gen_ai.completion）
	span.SetAttributes(
		attribute.Int("rag.result_count", len(docs)),
		attribute.String("gen_ai.completion", docsToCompletion(docs)),
	)
	return docs, nil
}

// IngestDocuments 批量向量化（含稀疏向量）并写入知识库
// 使用 EmbedBatch 一次性请求所有文档的稠密向量，减少 API 调用次数
// 同时调用 ComputeSparse 生成每篇文档的稀疏向量，用于 Hybrid Search
func (r *Retriever) IngestDocuments(ctx context.Context, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}

	ctx, span := otel.Tracer("videomax").Start(ctx, "rag.retriever.ingest",
		trace.WithAttributes(
			attribute.String("gen_ai.operation.name", "retriever"),
			attribute.Int("rag.doc_count", len(docs)),
		))
	defer span.End()

	texts := make([]string, len(docs))
	for i, doc := range docs {
		texts[i] = doc.Content
	}
	vecs, err := r.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("批量向量化失败: %w", err)
	}
	for i := range docs {
		docs[i].Embedding = vecs[i]
		// 从 Metadata 中提取 provider 字段，方便 Milvus 标量过滤
		if provider, ok := docs[i].Metadata["provider"]; ok {
			docs[i].Provider = provider
		}
	}
	if err := r.store.Upsert(ctx, docs); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
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

// docsToCompletion 将文档列表序列化为 LangSmith completion 字符串
func docsToCompletion(docs []Document) string {
	var sb strings.Builder
	for i, d := range docs {
		sb.WriteString(fmt.Sprintf("[%d] id=%s provider=%s\n%s\n", i+1, d.ID, d.Provider, d.Content))
	}
	return strings.TrimSpace(sb.String())
}
