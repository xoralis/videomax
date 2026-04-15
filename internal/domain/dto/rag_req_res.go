package dto

// RAGSearchRequest GET /api/rag/search 的查询参数
type RAGSearchRequest struct {
	Query string `form:"query" binding:"required"` // 检索词
	TopK  int    `form:"top_k"`                    // 返回条数，0 = 使用服务端默认值
}

// RAGSearchItem 单条检索结果
type RAGSearchItem struct {
	ID       string            `json:"id"`
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata"`
}

// RAGSearchResponse GET /api/rag/search 的响应体
type RAGSearchResponse struct {
	Code    int             `json:"code"`
	Msg     string          `json:"msg"`
	Results []RAGSearchItem `json:"results"`
}

// RAGIngestTextRequest POST /api/rag/ingest/text 的请求体（JSON）
type RAGIngestTextRequest struct {
	Documents []RAGIngestDoc `json:"documents" binding:"required,min=1"`
}

// RAGIngestDoc 单条文本入库项
type RAGIngestDoc struct {
	ID       string            `json:"id" binding:"required"`
	Content  string            `json:"content" binding:"required"`
	Metadata map[string]string `json:"metadata"`
}

// RAGIngestResponse 入库接口的通用响应体
type RAGIngestResponse struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Ingested int   `json:"ingested"` // 实际入库的文档/分块数量
}
