package rag

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

const (
	fieldID       = "id"
	fieldContent  = "content"
	fieldMetadata = "metadata"
	fieldVector   = "embedding"

	// maxContentLen metadata varchar 字段长度上限（Milvus 要求必须设最大长度）
	maxContentLen  = 4096
	maxMetadataLen = 1024
	maxIDLen       = 128
)

// MilvusStore 基于 Milvus Standalone 的向量存储实现
type MilvusStore struct {
	cli        client.Client
	collection string
	dim        int
}

// NewMilvusStore 创建并初始化 MilvusStore
// addr: Milvus gRPC 地址，如 "localhost:19530"
// collectionName: 集合名称，如 "videomax_knowledge"
// dim: 向量维度，需与 Embedder.Dim() 一致
func NewMilvusStore(ctx context.Context, addr, collectionName string, dim int) (*MilvusStore, error) {
	cli, err := client.NewClient(ctx, client.Config{Address: addr})
	if err != nil {
		return nil, fmt.Errorf("连接 Milvus 失败 (addr=%s): %w", addr, err)
	}

	store := &MilvusStore{
		cli:        cli,
		collection: collectionName,
		dim:        dim,
	}

	if err := store.ensureCollection(ctx); err != nil {
		_ = cli.Close()
		return nil, err
	}

	return store, nil
}

// ensureCollection 如果集合不存在则创建，存在则跳过（幂等）
func (s *MilvusStore) ensureCollection(ctx context.Context) error {
	exists, err := s.cli.HasCollection(ctx, s.collection)
	if err != nil {
		return fmt.Errorf("检查集合存在性失败: %w", err)
	}
	if exists {
		// 集合已存在，加载到内存以便查询
		if err := s.cli.LoadCollection(ctx, s.collection, false); err != nil {
			return fmt.Errorf("加载集合失败: %w", err)
		}
		return nil
	}

	// 定义 Schema
	schema := entity.NewSchema().WithName(s.collection).WithDescription("videoMax RAG 知识库").
		WithField(entity.NewField().WithName(fieldID).WithDataType(entity.FieldTypeVarChar).WithIsPrimaryKey(true).WithMaxLength(maxIDLen)).
		WithField(entity.NewField().WithName(fieldContent).WithDataType(entity.FieldTypeVarChar).WithMaxLength(maxContentLen)).
		WithField(entity.NewField().WithName(fieldMetadata).WithDataType(entity.FieldTypeVarChar).WithMaxLength(maxMetadataLen)).
		WithField(entity.NewField().WithName(fieldVector).WithDataType(entity.FieldTypeFloatVector).WithDim(int64(s.dim)))

	if err := s.cli.CreateCollection(ctx, schema, entity.DefaultShardNumber); err != nil {
		return fmt.Errorf("创建集合失败: %w", err)
	}

	// 为向量字段创建 HNSW 索引（M=16, efConstruction=256，适合中小规模知识库）
	idx, err := entity.NewIndexHNSW(entity.COSINE, 16, 256)
	if err != nil {
		return fmt.Errorf("构建索引参数失败: %w", err)
	}
	if err := s.cli.CreateIndex(ctx, s.collection, fieldVector, idx, false); err != nil {
		return fmt.Errorf("创建向量索引失败: %w", err)
	}

	// 加载到内存
	if err := s.cli.LoadCollection(ctx, s.collection, false); err != nil {
		return fmt.Errorf("加载集合失败: %w", err)
	}

	return nil
}

// Upsert 批量写入文档
// Milvus 不支持真正的 upsert，此处先 Delete 主键再 Insert（幂等语义）
func (s *MilvusStore) Upsert(ctx context.Context, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}

	ids := make([]string, 0, len(docs))
	contents := make([]string, 0, len(docs))
	metadatas := make([]string, 0, len(docs))
	vectors := make([][]float32, 0, len(docs))

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
	}

	// 先删除已有记录（若不存在则忽略错误）
	idFilter := buildIDFilter(ids)
	_ = s.cli.Delete(ctx, s.collection, "", idFilter)

	columns := []entity.Column{
		entity.NewColumnVarChar(fieldID, ids),
		entity.NewColumnVarChar(fieldContent, contents),
		entity.NewColumnVarChar(fieldMetadata, metadatas),
		entity.NewColumnFloatVector(fieldVector, s.dim, vectors),
	}

	if _, err := s.cli.Insert(ctx, s.collection, "", columns...); err != nil {
		return fmt.Errorf("插入文档失败: %w", err)
	}

	// 强制刷新，使数据可被查询
	if err := s.cli.Flush(ctx, s.collection, false); err != nil {
		return fmt.Errorf("flush 失败: %w", err)
	}

	return nil
}

// Search 查询与 query 向量最相似的 topK 条文档
func (s *MilvusStore) Search(ctx context.Context, query []float32, topK int) ([]Document, error) {
	// ef=64：搜索时探索的候选节点数，越大召回率越高但延迟也越高
	sp, err := entity.NewIndexHNSWSearchParam(64)
	if err != nil {
		return nil, fmt.Errorf("构建搜索参数失败: %w", err)
	}

	results, err := s.cli.Search(
		ctx,
		s.collection,
		nil,
		"",
		[]string{fieldID, fieldContent, fieldMetadata},
		[]entity.Vector{entity.FloatVector(query)},
		fieldVector,
		entity.COSINE,
		topK,
		sp,
	)
	if err != nil {
		return nil, fmt.Errorf("向量检索失败: %w", err)
	}

	docs := make([]Document, 0)
	for _, result := range results {
		for i := 0; i < result.ResultCount; i++ {
			doc := Document{}
			for _, col := range result.Fields {
				switch col.Name() {
				case fieldID:
					if v, ok := col.(*entity.ColumnVarChar); ok {
						doc.ID, _ = v.ValueByIdx(i)
					}
				case fieldContent:
					if v, ok := col.(*entity.ColumnVarChar); ok {
						doc.Content, _ = v.ValueByIdx(i)
					}
				case fieldMetadata:
					if v, ok := col.(*entity.ColumnVarChar); ok {
						raw, _ := v.ValueByIdx(i)
						_ = json.Unmarshal([]byte(raw), &doc.Metadata)
					}
				}
			}
			docs = append(docs, doc)
		}
	}

	return docs, nil
}

// Close 关闭 Milvus 客户端连接
func (s *MilvusStore) Close() error {
	return s.cli.Close()
}

// buildIDFilter 构造 Milvus 表达式：id in ["a","b","c"]
func buildIDFilter(ids []string) string {
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = fmt.Sprintf(`"%s"`, id)
	}
	result := fieldID + " in ["
	for i, q := range quoted {
		if i > 0 {
			result += ","
		}
		result += q
	}
	result += "]"
	return result
}

func truncateStr(s string, max int) string {
	runes := []rune(s)
	if len(runes) > max {
		return string(runes[:max])
	}
	return s
}
