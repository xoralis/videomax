// search_knowledge 交互式知识库检索示例
// 连接 Milvus 后循环读取用户输入，对每条 query 执行语义检索并打印结果
//
// 使用方法：
//
//	go run scripts/search_knowledge/main.go
//	go run scripts/search_knowledge/main.go -query "seedance 推荐分辨率"
//	go run scripts/search_knowledge/main.go -query "推荐分辨率" -topk 5
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"video-max/pkg/config"
	"video-max/pkg/logger"
	"video-max/pkg/rag"
)

func main() {
	query := flag.String("query", "", "直接指定检索词（留空则进入交互模式）")
	topK := flag.Int("topk", 0, "返回结果数量，0 = 使用 config.yaml 中的 top_k")
	flag.Parse()

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
		fmt.Fprintf(os.Stderr, "Milvus 连接失败: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	k := cfg.RAG.TopK
	if *topK > 0 {
		k = *topK
	}
	retriever := rag.NewRetriever(embedder, store, k)

	fmt.Printf("✅ 已连接 Milvus [%s / %s]，top_k=%d\n\n", cfg.RAG.MilvusAddr, cfg.RAG.Collection, k)

	// -query 参数模式：执行一次后退出
	if *query != "" {
		search(ctx, retriever, *query)
		return
	}

	// 交互模式：循环读取标准输入
	fmt.Println("进入交互模式，输入检索词后回车，输入 q 或 quit 退出")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "q" || input == "quit" {
			fmt.Println("退出")
			break
		}
		search(ctx, retriever, input)
	}
}

func search(ctx context.Context, retriever *rag.Retriever, query string) {
	docs, err := retriever.Retrieve(ctx, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "检索失败: %v\n", err)
		return
	}

	fmt.Printf("── 检索：%q ──\n", query)
	if len(docs) == 0 {
		fmt.Println("未找到相关结果")
		return
	}
	for i, doc := range docs {
		fmt.Printf("[%d] (source=%s)\n%s\n", i+1, doc.Metadata["source"], doc.Content)
		if i < len(docs)-1 {
			fmt.Println()
		}
	}
}
