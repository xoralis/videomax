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
	// Embedding 向量（由 Embedder 填充，只在写入流程中使用）
	Embedding []float32
}

// VectorStore 向量存储的统一接口
// 屏蔽底层向量库差异（Milvus / chromem-go 等），只暴露 Upsert 和 Search 两个操作
type VectorStore interface {
	// Upsert 批量写入或更新文档（ID 存在则覆盖，不存在则插入）
	Upsert(ctx context.Context, docs []Document) error
	// Search 根据查询向量检索最相似的 topK 条文档
	Search(ctx context.Context, query []float32, topK int) ([]Document, error)
	// Close 关闭连接，释放资源
	Close() error
}
