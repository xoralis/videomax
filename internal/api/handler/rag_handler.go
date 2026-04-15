package handler

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"video-max/internal/domain/dto"
	"video-max/pkg/logger"
	"video-max/pkg/rag"
)

// RAGHandler 知识库管理相关的 HTTP 接口控制器
// 暴露文档入库（文件上传 / 文本直接提交）和语义检索两类接口
type RAGHandler struct {
	retriever    *rag.Retriever
	chunkSize    int
	chunkOverlap int
}

// NewRAGHandler 创建 RAGHandler
// chunkSize / chunkOverlap 来自 cfg.RAG，用于文件入库时的分块参数
func NewRAGHandler(retriever *rag.Retriever, chunkSize, chunkOverlap int) *RAGHandler {
	if chunkSize <= 0 {
		chunkSize = 512
	}
	if chunkOverlap < 0 {
		chunkOverlap = 0
	}
	return &RAGHandler{
		retriever:    retriever,
		chunkSize:    chunkSize,
		chunkOverlap: chunkOverlap,
	}
}

// Search 语义检索
// GET /api/rag/search?query=xxx&top_k=3
func (h *RAGHandler) Search(c *gin.Context) {
	var req dto.RAGSearchRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.RAGSearchResponse{Code: -1, Msg: "参数错误: " + err.Error()})
		return
	}

	// top_k 可由前端覆盖，默认使用 Retriever 内置值
	retriever := h.retriever
	if req.TopK > 0 {
		retriever = retriever.WithTopK(req.TopK)
	}

	docs, err := retriever.Retrieve(c.Request.Context(), req.Query)
	if err != nil {
		logger.Log.Errorw("RAG 检索失败", "query", req.Query, "error", err)
		c.JSON(http.StatusInternalServerError, dto.RAGSearchResponse{Code: -1, Msg: "检索失败: " + err.Error()})
		return
	}

	items := make([]dto.RAGSearchItem, 0, len(docs))
	for _, d := range docs {
		items = append(items, dto.RAGSearchItem{
			ID:       d.ID,
			Content:  d.Content,
			Metadata: d.Metadata,
		})
	}
	c.JSON(http.StatusOK, dto.RAGSearchResponse{Code: 0, Msg: "ok", Results: items})
}

// IngestFile 上传文件并入库
// POST /api/rag/ingest/file  (multipart/form-data, 字段名: file)
// 可选 form 字段: source（来源标签，写入 metadata）
func (h *RAGHandler) IngestFile(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.RAGIngestResponse{Code: -1, Msg: "缺少 file 字段: " + err.Error()})
		return
	}

	// 将上传文件写入临时目录（保留原始扩展名，供 LoadFile 判断格式）
	tmpFile, err := os.CreateTemp("", "rag-upload-*"+filepath.Ext(fileHeader.Filename))
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.RAGIngestResponse{Code: -1, Msg: "临时文件创建失败"})
		return
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath) // 入库完成后清理临时文件

	if err := c.SaveUploadedFile(fileHeader, tmpPath); err != nil {
		c.JSON(http.StatusInternalServerError, dto.RAGIngestResponse{Code: -1, Msg: "文件保存失败: " + err.Error()})
		return
	}

	source := c.PostForm("source")
	if source == "" {
		source = fileHeader.Filename
	}

	docs, err := rag.LoadFile(c.Request.Context(), tmpPath, rag.LoaderOptions{
		ChunkSize:    h.chunkSize,
		ChunkOverlap: h.chunkOverlap,
		ExtraMetadata: map[string]string{
			"source_file": fileHeader.Filename,
			"source":      source,
		},
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.RAGIngestResponse{Code: -1, Msg: "文件解析失败: " + err.Error()})
		return
	}

	if err := h.retriever.IngestDocuments(c.Request.Context(), docs); err != nil {
		logger.Log.Errorw("RAG 文件入库失败", "file", fileHeader.Filename, "error", err)
		c.JSON(http.StatusInternalServerError, dto.RAGIngestResponse{Code: -1, Msg: "入库失败: " + err.Error()})
		return
	}

	logger.Log.Infow("RAG 文件入库成功", "file", fileHeader.Filename, "chunks", len(docs))
	c.JSON(http.StatusOK, dto.RAGIngestResponse{Code: 0, Msg: "入库成功", Ingested: len(docs)})
}

// IngestText 直接提交文本入库
// POST /api/rag/ingest/text  (application/json)
func (h *RAGHandler) IngestText(c *gin.Context) {
	var req dto.RAGIngestTextRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.RAGIngestResponse{Code: -1, Msg: "请求体格式错误: " + err.Error()})
		return
	}

	docs := make([]rag.Document, 0, len(req.Documents))
	for _, item := range req.Documents {
		meta := item.Metadata
		if meta == nil {
			meta = map[string]string{}
		}
		docs = append(docs, rag.Document{
			ID:       item.ID,
			Content:  item.Content,
			Metadata: meta,
		})
	}

	if err := h.retriever.IngestDocuments(c.Request.Context(), docs); err != nil {
		logger.Log.Errorw("RAG 文本入库失败", "error", err)
		c.JSON(http.StatusInternalServerError, dto.RAGIngestResponse{Code: -1, Msg: "入库失败: " + err.Error()})
		return
	}

	logger.Log.Infow("RAG 文本入库成功", "doc_count", len(docs))
	c.JSON(http.StatusOK, dto.RAGIngestResponse{Code: 0, Msg: "入库成功", Ingested: len(docs)})
}
