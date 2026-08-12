package knowledge

import (
	"strings"
)

// 分块参数（docs/KNOWLEDGE-BASE.md §2.6，QA-K10）。
const (
	// chunkMaxChars 无标题/超长兜底切分上限（字符）。
	chunkMaxChars = 800
	// chunkOverlap 兜底切分重叠（字符）。
	chunkOverlap = 100
)

// chunk 单个文本块（标题分块产物）。
type chunk struct {
	// Heading 所属标题（标题分块时记录；无标题空串）。
	Heading string
	// Content 块文本。
	Content string
}

// splitChunks 将文档文本分块（QA-K10）：
//
//   - Markdown 按标题分块：一个标题一段（标题行作 heading，正文归入该块；
//     块内再按 chunkMaxChars 兜底切分）。
//   - 无标题文档（非 Markdown / 无 `#`）按字符切分：≤ chunkMaxChars + 100 重叠。
//   - 标题分块内超长（超过 chunkMaxChars）→ 按该块内容独立切分（重叠 100）。
func splitChunks(content string) []chunk {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	// 按 Markdown 标题行切分（### / ## / #）。
	lines := strings.Split(content, "\n")
	var blocks []struct {
		heading string
		text    []string
	}
	curHeading := ""
	cur := []string{}
	for _, ln := range lines {
		if h, ok := markdownHeading(ln); ok {
			// 标题行本身并入新块（heading 记录标题文本）。
			if len(cur) > 0 || len(blocks) > 0 {
				blocks = append(blocks, struct {
					heading string
					text    []string
				}{heading: curHeading, text: cur})
			}
			curHeading = h
			cur = []string{ln}
			continue
		}
		cur = append(cur, ln)
	}
	if len(cur) > 0 || len(blocks) == 0 {
		blocks = append(blocks, struct {
			heading string
			text    []string
		}{heading: curHeading, text: cur})
	}

	var out []chunk
	for _, b := range blocks {
		text := strings.TrimSpace(strings.Join(b.text, "\n"))
		if text == "" {
			continue
		}
		// 块内超长（含标题块）→ 独立切分，重叠保留（QA-K10 大小兜底）。
		out = append(out, splitByLength(text, b.heading)...)
	}
	return out
}

// markdownHeading 判断行是否为 Markdown 标题，返回标题文本。
func markdownHeading(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	// 至少一个 # 后跟空格（#，##，###；## 锚点 "###" 不含空格不算标题）。
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level > 6 {
		return "", false
	}
	if level == 0 || level >= len(trimmed) || trimmed[level] != ' ' {
		return "", false
	}
	return strings.TrimSpace(trimmed[level:]), true
}

// splitByLength 按字符切分（≤ chunkMaxChars，重叠 chunkOverlap）。
func splitByLength(text, heading string) []chunk {
	runes := []rune(text)
	if len(runes) <= chunkMaxChars {
		return []chunk{{Heading: heading, Content: text}}
	}
	var out []chunk
	start := 0
	for start < len(runes) {
		end := start + chunkMaxChars
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, chunk{Heading: heading, Content: string(runes[start:end])})
		if end == len(runes) {
			break
		}
		start = end - chunkOverlap
	}
	return out
}
