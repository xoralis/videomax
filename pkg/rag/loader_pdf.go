package rag

import (
	"context"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// PDFLoader PDF 文件加载器（.pdf）
// 使用纯 Go 实现的 ledongthuc/pdf 提取每页文本，无需 CGO 或外部依赖
// 提取后按 RecursiveChunker（段落→行→空格→字符）递归切分
type PDFLoader struct {
	Options LoaderOptions
}

func (l *PDFLoader) Load(_ context.Context, path string) ([]Document, error) {
	text, pageCount, err := extractPDFText(path)
	if err != nil {
		return nil, fmt.Errorf("PDF 文本提取失败 (%s): %w", path, err)
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("PDF 未提取到任何文本，可能是扫描件或加密文件: %s", path)
	}

	chunker := &RecursiveChunker{
		ChunkSize:    l.Options.ChunkSize,
		ChunkOverlap: l.Options.ChunkOverlap,
		// PDF 从段落级别开始递归
		Separators: []string{"\n\n", "\n", " ", ""},
	}

	meta := map[string]string{
		"format":     "pdf",
		"page_count": fmt.Sprintf("%d", pageCount),
	}
	for k, v := range l.Options.ExtraMetadata {
		meta[k] = v
	}

	return chunker.Chunk(text, path, meta), nil
}

// extractPDFText 从 PDF 中提取所有页的纯文本
// 返回完整文本和总页数
func extractPDFText(path string) (string, int, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	totalPages := r.NumPage()
	var sb strings.Builder

	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			// 单页提取失败时跳过，不中断整体流程
			continue
		}
		sb.WriteString(text)
		// 页间加换行，便于段落切分时不跨页合并
		if pageNum < totalPages {
			sb.WriteString("\n\n")
		}
	}

	return sb.String(), totalPages, nil
}
