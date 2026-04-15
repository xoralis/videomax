package rag

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// DocumentLoader 文档加载接口
// 将磁盘上的各种格式文件读取并分块，输出可直接入库的 []Document（不含 Embedding）
// Embedding 由 Retriever.IngestDocuments 在入库时统一计算
type DocumentLoader interface {
	Load(ctx context.Context, path string) ([]Document, error)
}

// LoaderOptions 加载选项
type LoaderOptions struct {
	// ChunkSize 每块目标字符数（Rune），默认 512
	ChunkSize int
	// ChunkOverlap 相邻块重叠字符数，默认 50
	ChunkOverlap int
	// ExtraMetadata 附加到每条 Document.Metadata 的自定义键值对
	ExtraMetadata map[string]string
}

func defaultOptions(opts LoaderOptions) LoaderOptions {
	if opts.ChunkSize <= 0 {
		opts.ChunkSize = 512
	}
	if opts.ChunkOverlap < 0 {
		opts.ChunkOverlap = 0
	}
	if opts.ExtraMetadata == nil {
		opts.ExtraMetadata = map[string]string{}
	}
	return opts
}

// LoadFile 根据文件扩展名自动选择合适的 Loader 并加载
// 支持扩展名：.txt .md .markdown .pdf
func LoadFile(ctx context.Context, path string, opts LoaderOptions) ([]Document, error) {
	opts = defaultOptions(opts)

	ext := strings.ToLower(filepath.Ext(path))
	var loader DocumentLoader
	switch ext {
	case ".txt":
		loader = &TextLoader{Options: opts}
	case ".md", ".markdown":
		loader = &MarkdownLoader{Options: opts}
	// TODO PDF解析暂未实现
	case ".pdf":
		loader = &PDFLoader{Options: opts}
	default:
		return nil, fmt.Errorf("不支持的文件格式 %q，当前支持: .txt .md .markdown .pdf", ext)
	}

	return loader.Load(ctx, path)
}
