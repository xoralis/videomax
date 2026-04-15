package rag

import (
	"context"
	"os"
	"regexp"
	"strings"
)

// TextLoader 纯文本文件加载器（.txt）
type TextLoader struct {
	Options LoaderOptions
}

func (l *TextLoader) Load(_ context.Context, path string) ([]Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	chunker := &TextChunker{
		ChunkSize:    l.Options.ChunkSize,
		ChunkOverlap: l.Options.ChunkOverlap,
	}
	return chunker.Chunk(string(data), path, l.Options.ExtraMetadata), nil
}

// MarkdownLoader Markdown 文件加载器（.md / .markdown）
// 采用递归切分策略：先按标题层级（H1→H2→...→段落→行）递归拆分原始 Markdown，
// 保留每块的标题上下文；切分完成后对每块单独去除 Markdown 语法符号后入库
type MarkdownLoader struct {
	Options LoaderOptions
}

var (
	// 匹配 Markdown 标题行（# 至 ######）
	reHeading = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	// 匹配行内代码、加粗、斜体、删除线等常见 Markdown 符号
	reInlineMarkup = regexp.MustCompile(`[*_~` + "`" + `]{1,3}`)
	// 匹配 [文字](链接) 和 ![图片](链接)
	reLink = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	// 匹配代码块
	reCodeBlock = regexp.MustCompile("(?s)```[^`]*```")
)

func (l *MarkdownLoader) Load(_ context.Context, path string) ([]Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	chunker := &RecursiveChunker{
		ChunkSize:    l.Options.ChunkSize,
		ChunkOverlap: l.Options.ChunkOverlap,
		Separators:   MarkdownSeparators,
	}

	meta := map[string]string{"format": "markdown"}
	for k, v := range l.Options.ExtraMetadata {
		meta[k] = v
	}

	// 先对原始 Markdown 递归切分（标题分隔符 \n## 等必须在切分前存在）
	docs := chunker.Chunk(string(data), path, meta)

	// 对每块单独清理 Markdown 符号，使入库文本干净且利于向量化
	for i := range docs {
		docs[i].Content = strings.TrimSpace(stripMarkdown(docs[i].Content))
	}
	return docs, nil
}

// stripMarkdown 将 Markdown 文本转换为纯文本
// 保留文字内容，去除格式符号，使向量化结果更准确
func stripMarkdown(src string) string {
	// 先去除代码块（保留其中纯文本）
	src = reCodeBlock.ReplaceAllStringFunc(src, func(block string) string {
		// 去掉首尾 ``` 行
		lines := strings.Split(block, "\n")
		if len(lines) > 2 {
			return strings.Join(lines[1:len(lines)-1], "\n")
		}
		return ""
	})
	// 去除行内代码（`code`）
	src = regexp.MustCompile("`[^`]*`").ReplaceAllStringFunc(src, func(s string) string {
		return strings.Trim(s, "`")
	})
	// 标题行去除 # 符号
	src = reHeading.ReplaceAllString(src, "")
	// 链接只保留文字
	src = reLink.ReplaceAllString(src, "$1")
	// 去除加粗、斜体等符号
	src = reInlineMarkup.ReplaceAllString(src, "")
	// 合并多余空行
	src = regexp.MustCompile(`\n{3,}`).ReplaceAllString(src, "\n\n")
	return strings.TrimSpace(src)
}
