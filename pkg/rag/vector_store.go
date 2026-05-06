package rag

import "context"

// Document 向量库中存储的文档单元
type Document struct {
	// ID 文档唯一标识（如 "bytedance_resolution", "task_uuid_xxx"）
	ID string
	// Content 原始文本内容，将被 Embedder 向量化并存入向量库
	Content string
	// Metadata 附加的元数据，以 JSON 字符串形式存入 Milvus
	// 例如: {"provider":"bytedance","category":"resolution","task_id":"xxx"}
	Metadata map[string]string
	// Embedding 稠密向量（由 Embedder 填充，只在写入流程中使用）
	Embedding []float32
	// Provider 视频供应商标识（如 "bytedance", "kling"），用于标量过滤检索
	// 与 Metadata["provider"] 保持一致，单独存储以便 Milvus expr 过滤
	Provider string
}

// VectorStore 向量存储的统一接口
// 屏蔽底层向量库差异（Milvus / chromem-go 等），只暴露 Upsert 和 Search 两个操作
type VectorStore interface {
	// Upsert 批量写入或更新文档（ID 存在则覆盖，不存在则插入）
	Upsert(ctx context.Context, docs []Document) error
	// Search 根据查询向量检索最相似的 topK 条文档
	// queryText: 原始查询文本，用于 BM25 稀疏检索；filter: Milvus 标量过滤表达式，空字符串表示不过滤
	Search(ctx context.Context, query []float32, queryText string, topK int, filter string) ([]Document, error)
	// Close 关闭连接，释放资源
	Close() error
}

