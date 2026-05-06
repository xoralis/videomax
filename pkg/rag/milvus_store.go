package rag

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"video-max/pkg/logger"

	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

const (
	fieldID       = "id"
	fieldContent  = "content"
	fieldMetadata = "metadata"
	fieldVector   = "embedding"
	fieldSparse   = "sparse_embedding"
	fieldProvider = "provider"

	maxContentLen  = 4096
	maxMetadataLen = 1024
	maxIDLen       = 128
	maxProviderLen = 64
)

// MilvusStore 基于 Milvus 2.5 Standalone 的向量存储实现
// 使用 Milvus 原生 BM25 Function 进行服务端稀疏向量生成，客户端无需计算稀疏向量
type MilvusStore struct {
	cli        *milvusclient.Client
	collection string
	dim        int
}

// NewMilvusStore 创建并初始化 MilvusStore
// addr: Milvus gRPC 地址，如 "localhost:19530"
// collectionName: 集合名称，如 "videomax_knowledge"
// dim: 向量维度，需与 Embedder.Dim() 一致
func NewMilvusStore(ctx context.Context, addr, collectionName string, dim int) (*MilvusStore, error) {
	cli, err := milvusclient.New(ctx, &milvusclient.ClientConfig{Address: addr})
	if err != nil {
		return nil, fmt.Errorf("连接 Milvus 失败 (addr=%s): %w", addr, err)
	}

	store := &MilvusStore{
		cli:        cli,
		collection: collectionName,
		dim:        dim,
	}

	if err := store.ensureCollection(ctx); err != nil {
		_ = cli.Close(ctx)
		return nil, err
	}

	return store, nil
}

// ensureCollection 如果集合不存在则创建（含 BM25 Function），存在则加载
func (s *MilvusStore) ensureCollection(ctx context.Context) error {
	exists, err := s.cli.HasCollection(ctx, milvusclient.NewHasCollectionOption(s.collection))
	if err != nil {
		return fmt.Errorf("检查集合存在性失败: %w", err)
	}

	if exists {
		loadTask, err := s.cli.LoadCollection(ctx, milvusclient.NewLoadCollectionOption(s.collection))
		if err != nil {
			return fmt.Errorf("加载集合失败: %w", err)
		}
		if err := loadTask.Await(ctx); err != nil {
			return fmt.Errorf("等待集合加载失败: %w", err)
		}
		logger.Log.Infow("Milvus 集合已加载", "collection", s.collection)
		return nil
	}

	// ── 创建新集合（含 BM25 Function）──
	schema := entity.NewSchema().
		WithName(s.collection).
		WithDescription("videoMax RAG 知识库（Hybrid BM25）").
		WithField(entity.NewField().WithName(fieldID).WithDataType(entity.FieldTypeVarChar).WithIsPrimaryKey(true).WithMaxLength(maxIDLen)).
		WithField(entity.NewField().WithName(fieldContent).WithDataType(entity.FieldTypeVarChar).WithMaxLength(maxContentLen).WithEnableAnalyzer(true)).
		WithField(entity.NewField().WithName(fieldMetadata).WithDataType(entity.FieldTypeVarChar).WithMaxLength(maxMetadataLen)).
		WithField(entity.NewField().WithName(fieldVector).WithDataType(entity.FieldTypeFloatVector).WithDim(int64(s.dim))).
		WithField(entity.NewField().WithName(fieldSparse).WithDataType(entity.FieldTypeSparseVector)).
		WithField(entity.NewField().WithName(fieldProvider).WithDataType(entity.FieldTypeVarChar).WithMaxLength(maxProviderLen)).
		WithFunction(entity.NewFunction().
			WithName("bm25_fn").
			WithType(entity.FunctionTypeBM25).
			WithInputFields(fieldContent).
			WithOutputFields(fieldSparse))

	if err := s.cli.CreateCollection(ctx, milvusclient.NewCreateCollectionOption(s.collection, schema)); err != nil {
		return fmt.Errorf("创建集合失败: %w", err)
	}

	// 稠密向量 HNSW 索引
	denseIdx := index.NewHNSWIndex(entity.COSINE, 16, 256)
	denseTask, err := s.cli.CreateIndex(ctx, milvusclient.NewCreateIndexOption(s.collection, fieldVector, denseIdx))
	if err != nil {
		return fmt.Errorf("创建稠密向量索引失败: %w", err)
	}
	if err := denseTask.Await(ctx); err != nil {
		return fmt.Errorf("等待稠密索引创建失败: %w", err)
	}

	// 稀疏向量 BM25 索引
	sparseIdx := index.NewSparseInvertedIndex(entity.BM25, 0.1)
	sparseTask, err := s.cli.CreateIndex(ctx, milvusclient.NewCreateIndexOption(s.collection, fieldSparse, sparseIdx))
	if err != nil {
		return fmt.Errorf("创建稀疏向量索引失败: %w", err)
	}
	if err := sparseTask.Await(ctx); err != nil {
		return fmt.Errorf("等待稀疏索引创建失败: %w", err)
	}

	loadTask, err := s.cli.LoadCollection(ctx, milvusclient.NewLoadCollectionOption(s.collection))
	if err != nil {
		return fmt.Errorf("加载集合失败: %w", err)
	}
	if err := loadTask.Await(ctx); err != nil {
		return fmt.Errorf("等待集合加载失败: %w", err)
	}

	logger.Log.Infow("Milvus 新集合创建成功，Hybrid BM25 Search 已启用", "collection", s.collection)
	return nil
}

// Upsert 批量写入文档（先删后插，幂等语义）
// 注意：无需传入稀疏向量，Milvus BM25 Function 自动从 content 字段生成
func (s *MilvusStore) Upsert(ctx context.Context, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}

	ctx, span := otel.Tracer("videomax").Start(ctx, "rag.milvus.upsert",
		trace.WithAttributes(
			attribute.String("gen_ai.operation.name", "retriever"),
			attribute.String("db.system", "milvus"),
			attribute.String("db.collection", s.collection),
			attribute.Int("rag.doc_count", len(docs)),
		))
	defer span.End()

	ids := make([]string, 0, len(docs))
	contents := make([]string, 0, len(docs))
	metadatas := make([]string, 0, len(docs))
	vectors := make([][]float32, 0, len(docs))
	providers := make([]string, 0, len(docs))

	for _, doc := range docs {
		if doc.Embedding == nil {
			return fmt.Errorf("文档 '%s' 缺少 Embedding 向量，请先调用 Embedder.Embed", doc.ID)
		}
		metaJSON, err := json.Marshal(doc.Metadata)
		if err != nil {
			return fmt.Errorf("序列化 metadata 失败: %w", err)
		}

		ids = append(ids, truncateStr(doc.ID, maxIDLen))
		contents = append(contents, truncateStr(doc.Content, maxContentLen))
		metadatas = append(metadatas, truncateStr(string(metaJSON), maxMetadataLen))
		vectors = append(vectors, doc.Embedding)
		providers = append(providers, truncateStr(doc.Provider, maxProviderLen))
	}

	// 先删除已有记录（若不存在则忽略错误）
	_, _ = s.cli.Delete(ctx, milvusclient.NewDeleteOption(s.collection).WithStringIDs(fieldID, ids))

	if _, err := s.cli.Upsert(ctx, milvusclient.NewColumnBasedInsertOption(s.collection,
		column.NewColumnVarChar(fieldID, ids),
		column.NewColumnVarChar(fieldContent, contents),
		column.NewColumnVarChar(fieldMetadata, metadatas),
		column.NewColumnFloatVector(fieldVector, s.dim, vectors),
		column.NewColumnVarChar(fieldProvider, providers),
	)); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("写入文档失败: %w", err)
	}

	// Milvus 会自动定期 flush，无需手动调用（手动 flush 受 gRPC RateLimiter 限制 rate=0.1）
	// flushTask, err := s.cli.Flush(ctx, milvusclient.NewFlushOption(s.collection))
	// if err != nil {
	// 	span.RecordError(err)
	// 	span.SetStatus(codes.Error, err.Error())
	// 	return fmt.Errorf("flush 失败: %w", err)
	// }
	// if err := flushTask.Await(ctx); err != nil {
	// 	span.RecordError(err)
	// 	span.SetStatus(codes.Error, err.Error())
	// 	return fmt.Errorf("等待 flush 完成失败: %w", err)
	// }

	return nil
}

// Search 检索最相似的 topK 条文档（使用 dense+BM25 Hybrid Search + RRF 融合）
// query: 稠密查询向量；queryText: 原始查询文本（BM25 稀疏检索使用）
// filter: Milvus 标量过滤表达式，如 `provider == "bytedance"`；空字符串表示不过滤
func (s *MilvusStore) Search(ctx context.Context, query []float32, queryText string, topK int, filter string) ([]Document, error) {
	ctx, span := otel.Tracer("videomax").Start(ctx, "rag.milvus.search",
		trace.WithAttributes(
			attribute.String("gen_ai.operation.name", "retriever"),
			attribute.String("db.system", "milvus"),
			attribute.String("db.collection", s.collection),
			attribute.Int("rag.top_k", topK),
			attribute.String("rag.filter", filter),
			attribute.String("gen_ai.prompt", queryText),
		))
	defer span.End()

	denseReq := milvusclient.NewAnnRequest(fieldVector, topK, entity.FloatVector(query)).
		WithAnnParam(index.NewHNSWAnnParam(64))
	sparseReq := milvusclient.NewAnnRequest(fieldSparse, topK, entity.Text(queryText)).
		WithAnnParam(index.NewSparseAnnParam())

	if filter != "" {
		denseReq = denseReq.WithFilter(filter)
		sparseReq = sparseReq.WithFilter(filter)
	}

	results, err := s.cli.HybridSearch(ctx,
		milvusclient.NewHybridSearchOption(s.collection, topK, denseReq, sparseReq).
			WithReranker(milvusclient.NewRRFReranker()).
			WithOutputFields(fieldID, fieldContent, fieldMetadata, fieldProvider))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("Hybrid 检索失败: %w", err)
	}

	docs := extractDocs(results)
	span.SetAttributes(
		attribute.Int("rag.result_count", len(docs)),
		attribute.String("gen_ai.completion", docsToCompletion(docs)),
	)
	return docs, nil
}

// Close 关闭 Milvus 客户端连接
func (s *MilvusStore) Close() error {
	return s.cli.Close(context.Background())
}

// ── 内部工具函数 ──────────────────────────────────────────────────────────────

// extractDocs 从 HybridSearch 结果中提取 Document 列表
func extractDocs(results []milvusclient.ResultSet) []Document {
	docs := make([]Document, 0)
	for _, rs := range results {
		for i := 0; i < rs.ResultCount; i++ {
			doc := Document{}

			if col := rs.GetColumn(fieldID); col != nil {
				if c, ok := col.(*column.ColumnVarChar); ok {
					doc.ID, _ = c.Value(i)
				}
			}
			if col := rs.GetColumn(fieldContent); col != nil {
				if c, ok := col.(*column.ColumnVarChar); ok {
					doc.Content, _ = c.Value(i)
				}
			}
			if col := rs.GetColumn(fieldMetadata); col != nil {
				if c, ok := col.(*column.ColumnVarChar); ok {
					raw, _ := c.Value(i)
					_ = json.Unmarshal([]byte(raw), &doc.Metadata)
				}
			}
			if col := rs.GetColumn(fieldProvider); col != nil {
				if c, ok := col.(*column.ColumnVarChar); ok {
					doc.Provider, _ = c.Value(i)
				}
			}

			docs = append(docs, doc)
		}
	}
	return docs
}

func truncateStr(s string, max int) string {
	runes := []rune(s)
	if len(runes) > max {
		return string(runes[:max])
	}
	return s
}
