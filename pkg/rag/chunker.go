package rag

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// MarkdownSeparators Markdown 递归切分的分隔符优先级列表
// 从最粗粒度（一级标题）到最细粒度（字符），递归时依次降级
var MarkdownSeparators = []string{
	"\n# ",
	"\n## ",
	"\n### ",
	"\n#### ",
	"\n##### ",
	"\n######",
	"\n\n", // 段落
	"\n",   // 换行
	" ",    // 空格
	"",     // 兜底：按字符强制切分
}

// RecursiveChunker 递归文本分块器
// 按分隔符优先级从粗到细递归切分，保证每块不超过 ChunkSize
// 相邻块之间保留 ChunkOverlap 字符的重叠以延续上下文
type RecursiveChunker struct {
	ChunkSize    int
	ChunkOverlap int
	Separators   []string
}

// Chunk 对 text 执行递归切分，返回 []Document（不含 Embedding）
func (c *RecursiveChunker) Chunk(text, sourceID string, extraMeta map[string]string) []Document {
	pieces := c.splitText(text, c.Separators)

	var docs []Document
	chunkIdx := 0
	for _, piece := range pieces {
		piece = strings.TrimSpace(piece)
		if piece == "" {
			continue
		}
		meta := map[string]string{
			"source":      sourceID,
			"chunk_index": fmt.Sprintf("%d", chunkIdx),
		}
		for k, v := range extraMeta {
			meta[k] = v
		}
		docs = append(docs, Document{
			ID:       chunkID(sourceID, chunkIdx),
			Content:  piece,
			Metadata: meta,
		})
		chunkIdx++
	}
	return docs
}

// splitText 核心递归逻辑
func (c *RecursiveChunker) splitText(text string, separators []string) []string {
	// 已经足够小，直接返回
	if runeLen(text) <= c.ChunkSize {
		return []string{text}
	}

	// 没有分隔符可用，兜底按字符切
	if len(separators) == 0 {
		return c.forceSplit(text)
	}

	sep := separators[0]
	remaining := separators[1:]

	// 空字符串分隔符 = 按字符强制切分
	if sep == "" {
		return c.forceSplit(text)
	}

	// 当前分隔符在文本中不存在，尝试下一级
	if !strings.Contains(text, sep) {
		return c.splitText(text, remaining)
	}

	// 切分，并将分隔符追加回（除首段外），保留标题等结构内容
	parts := strings.Split(text, sep)
	splits := make([]string, 0, len(parts))
	splits = append(splits, parts[0])
	for _, p := range parts[1:] {
		splits = append(splits, sep+p)
	}

	// 分拣：小块直接收集；大块递归处理
	var goodSplits []string
	var result []string
	for _, split := range splits {
		if runeLen(split) <= c.ChunkSize {
			goodSplits = append(goodSplits, split)
		} else {
			if len(goodSplits) > 0 {
				result = append(result, c.mergeSplits(goodSplits)...)
				goodSplits = nil
			}
			if len(remaining) == 0 {
				result = append(result, c.forceSplit(split)...)
			} else {
				result = append(result, c.splitText(split, remaining)...)
			}
		}
	}
	if len(goodSplits) > 0 {
		result = append(result, c.mergeSplits(goodSplits)...)
	}
	return result
}

// mergeSplits 将若干小片段拼合成不超过 ChunkSize 的块，相邻块保留 ChunkOverlap 重叠
func (c *RecursiveChunker) mergeSplits(splits []string) []string {
	var result []string
	var current []string
	currentLen := 0

	for _, s := range splits {
		sLen := runeLen(s)
		if currentLen+sLen > c.ChunkSize && len(current) > 0 {
			merged := strings.Join(current, "")
			result = append(result, merged)

			// 从已合并文本的尾部取 ChunkOverlap 个字符作为下一块的前缀
			// 这样无论单个 split 多大，overlap 都能精确保留
			current = nil
			currentLen = 0
			if c.ChunkOverlap > 0 {
				runes := []rune(merged)
				overlapStart := len(runes) - c.ChunkOverlap
				if overlapStart < 0 {
					overlapStart = 0
				}
				if tail := string(runes[overlapStart:]); tail != "" {
					current = []string{tail}
					currentLen = runeLen(tail)
				}
			}
		}
		current = append(current, s)
		currentLen += sLen
	}
	if len(current) > 0 {
		if merged := strings.Join(current, ""); strings.TrimSpace(merged) != "" {
			result = append(result, merged)
		}
	}
	return result
}

// forceSplit 按字符数强制切分（无明显语义边界时的最后手段）
func (c *RecursiveChunker) forceSplit(text string) []string {
	runes := []rune(text)
	step := c.ChunkSize - c.ChunkOverlap
	if step <= 0 {
		step = c.ChunkSize
	}
	var result []string
	for start := 0; start < len(runes); start += step {
		end := start + c.ChunkSize
		if end > len(runes) {
			end = len(runes)
		}
		result = append(result, string(runes[start:end]))
		if end == len(runes) {
			break
		}
	}
	return result
}

func runeLen(s string) int { return len([]rune(s)) }

// TextChunker 将长文本按固定字符数分块，相邻块有重叠以保留上下文
type TextChunker struct {
	ChunkSize    int // 每块目标字符数（Rune），不含重叠部分
	ChunkOverlap int // 相邻块重叠字符数
}

// Chunk 将 text 分块并构造 []Document
// sourceID: 用于生成 Document.ID 的文件来源标识（如文件路径）
// extraMeta: 附加到每条 Document.Metadata 的自定义键值对
func (c *TextChunker) Chunk(text, sourceID string, extraMeta map[string]string) []Document {
	runes := []rune(text)
	total := len(runes)
	if total == 0 {
		return nil
	}

	step := c.ChunkSize - c.ChunkOverlap
	if step <= 0 {
		step = c.ChunkSize
	}

	var docs []Document
	chunkIdx := 0
	for start := 0; start < total; start += step {
		end := start + c.ChunkSize
		if end > total {
			end = total
		}

		content := string(runes[start:end])
		id := chunkID(sourceID, chunkIdx)

		meta := map[string]string{
			"source":      sourceID,
			"chunk_index": fmt.Sprintf("%d", chunkIdx),
		}
		for k, v := range extraMeta {
			meta[k] = v
		}

		docs = append(docs, Document{
			ID:       id,
			Content:  content,
			Metadata: meta,
		})
		chunkIdx++

		if end == total {
			break
		}
	}
	return docs
}

// chunkID 生成确定性的 Document ID：source 哈希前缀 + chunk 序号
func chunkID(source string, idx int) string {
	h := sha256.Sum256([]byte(source))
	return fmt.Sprintf("%x_%d", h[:4], idx)
}
