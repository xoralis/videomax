// ingest_knowledge 是一次性运行的知识库入库脚本
// 将供应商最佳实践规则向量化并写入 Milvus，供 RAGSearchTool 在 ReAct 循环中检索
//
// 使用方法：
//
//	go run scripts/ingest_knowledge/main.go
package main

import (
	"context"
	"fmt"
	"os"

	"video-max/pkg/config"
	"video-max/pkg/logger"
	"video-max/pkg/rag"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}
	if err := logger.Init(cfg.Log); err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败: %v\n", err)
		os.Exit(1)
	}

	// 初始化 Embedder
	embedAPIKey := cfg.RAG.EmbedAPIKey
	if embedAPIKey == "" {
		embedAPIKey = cfg.LLM.APIKey
	}
	embedBaseURL := cfg.RAG.EmbedBaseURL
	if embedBaseURL == "" {
		embedBaseURL = cfg.LLM.BaseURL
	}
	embedder := rag.NewEmbedder(embedAPIKey, embedBaseURL, cfg.RAG.EmbedModel, cfg.RAG.EmbedDim)

	// 连接 Milvus
	store, err := rag.NewMilvusStore(ctx, cfg.RAG.MilvusAddr, cfg.RAG.Collection, embedder.Dim())
	if err != nil {
		logger.Log.Fatalw("Milvus 连接失败", "error", err)
	}
	defer store.Close()

	retriever := rag.NewRetriever(embedder, store, cfg.RAG.TopK)

	// ==================== 知识库文档 ====================
	docs := []rag.Document{
		// ── 字节跳动 Seedance ──────────────────────────────
		{
			ID:       "bytedance_resolution",
			Content:  "bytedance Seedance 推荐分辨率: 1920x1080 (16:9) 或 1080x1920 (9:16)，最大支持 4K。",
			Metadata: map[string]string{"provider": "bytedance", "category": "resolution"},
		},
		{
			ID:       "bytedance_duration",
			Content:  "bytedance Seedance 支持视频时长: 5秒 或 10秒，推荐使用 5 秒获得最佳质量。",
			Metadata: map[string]string{"provider": "bytedance", "category": "duration"},
		},
		{
			ID:       "bytedance_style",
			Content:  "bytedance Seedance 支持的风格关键词: cinematic, anime, realistic, watercolor, cyberpunk, fantasy。",
			Metadata: map[string]string{"provider": "bytedance", "category": "style"},
		},
		{
			ID:       "bytedance_camera",
			Content:  "bytedance Seedance 推荐运镜关键词: tracking shot, dolly zoom, pan left/right, tilt up/down, static, aerial view。",
			Metadata: map[string]string{"provider": "bytedance", "category": "camera"},
		},
		{
			ID:       "bytedance_best_practice",
			Content:  "bytedance Seedance 提示词最佳实践: 1.主体描述放最前面 2.运镜指令紧跟其后 3.风格和光影放最后 4.避免否定词 5.结尾统一加 'high quality, 4K'。",
			Metadata: map[string]string{"provider": "bytedance", "category": "best_practice"},
		},

		// ── 快手 Kling ─────────────────────────────────────
		{
			ID:       "kling_resolution",
			Content:  "kling 推荐分辨率: 1280x720 或 1920x1080。",
			Metadata: map[string]string{"provider": "kling", "category": "resolution"},
		},
		{
			ID:       "kling_duration",
			Content:  "kling 支持视频时长: 5秒、10秒，专业版支持延长至 30 秒。",
			Metadata: map[string]string{"provider": "kling", "category": "duration"},
		},
		{
			ID:       "kling_style",
			Content:  "kling 支持的风格关键词: realistic, cartoon, oil painting, 3d render。",
			Metadata: map[string]string{"provider": "kling", "category": "style"},
		},
		{
			ID:       "kling_camera",
			Content:  "kling 推荐运镜关键词: push in, pull out, orbit, static, handheld。",
			Metadata: map[string]string{"provider": "kling", "category": "camera"},
		},
		{
			ID:       "kling_best_practice",
			Content:  "kling 提示词最佳实践: 1.使用中文效果更佳 2.画面主体+动作+环境三段式结构 3.添加 'high quality, 4K' 等质量词。",
			Metadata: map[string]string{"provider": "kling", "category": "best_practice"},
		},

		// ── 腾讯混元 Hunyuan ───────────────────────────────
		{
			ID:       "hunyuan_resolution",
			Content:  "hunyuan 混元视频推荐分辨率: 1280x720 (16:9) 或 720x1280 (9:16)。",
			Metadata: map[string]string{"provider": "hunyuan", "category": "resolution"},
		},
		{
			ID:       "hunyuan_duration",
			Content:  "hunyuan 混元视频支持时长: 5秒，生成速度约 2-5 分钟。",
			Metadata: map[string]string{"provider": "hunyuan", "category": "duration"},
		},
		{
			ID:       "hunyuan_best_practice",
			Content:  "hunyuan 混元视频提示词最佳实践: 支持纯中文描述，画面细节越具体质量越高，推荐加入光线描述（如：阳光照射、霓虹灯光）。",
			Metadata: map[string]string{"provider": "hunyuan", "category": "best_practice"},
		},

		// ── 通用提示词工程知识 ─────────────────────────────
		{
			ID:       "general_prompt_structure",
			Content:  "视频生成提示词通用结构: [角色外貌] + [动作序列，用 Initially/Then/Finally 连接] + [运镜描述] + [环境与光线] + [画质修饰词，放最后，只写一次]。",
			Metadata: map[string]string{"provider": "general", "category": "prompt_structure"},
		},
		{
			ID:       "general_character_anchor",
			Content:  "角色一致性技巧: 在提示词中重复使用相同的外貌锚点词（如 'short black hair, red hoodie'），确保多镜头角色外貌一致。",
			Metadata: map[string]string{"provider": "general", "category": "character_consistency"},
		},
		{
			ID:       "general_quality_words",
			Content:  "常用画质修饰词（放提示词末尾，只出现一次）: high quality, 4K resolution, ultra-detailed, cinematic lighting, sharp focus, smooth motion。",
			Metadata: map[string]string{"provider": "general", "category": "quality"},
		},
	}

	logger.Log.Infow("开始批量入库知识文档", "doc_count", len(docs))
	// if err := retriever.IngestDocuments(ctx, docs); err != nil {
	// 	logger.Log.Fatalw("知识入库失败", "error", err)
	// }
	logger.Log.Infow("✅ 硬编码知识入库完成", "doc_count", len(docs))

	// ==================== 从 Markdown 文件入库 ====================
	mdFiles := []struct {
		path     string
		metadata map[string]string
	}{
		{
			path:     `D:\MarkDown_Files\Go\videoMax\seedance.md`,
			metadata: map[string]string{"provider": "bytedance", "source_file": "seedance.md"},
		},
	}

	for _, f := range mdFiles {
		mdDocs, err := rag.LoadFile(ctx, f.path, rag.LoaderOptions{
			ChunkSize:     cfg.RAG.ChunkSize,
			ChunkOverlap:  cfg.RAG.ChunkOverlap,
			ExtraMetadata: f.metadata,
		})
		if err != nil {
			logger.Log.Errorw("Markdown 文件加载失败，跳过", "path", f.path, "error", err)
			continue
		}
		if err := retriever.IngestDocuments(ctx, mdDocs); err != nil {
			logger.Log.Errorw("Markdown 文档入库失败", "path", f.path, "error", err)
			continue
		}
		logger.Log.Infow("✅ Markdown 文件入库完成", "path", f.path, "chunk_count", len(mdDocs))
	}

	logger.Log.Infow("✅ 全部入库完成", "collection", cfg.RAG.Collection)
}
