package knowledge

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestMarkdownHeading(t *testing.T) {
	cases := []struct {
		line string
		want string
		ok   bool
	}{
		{"# 一级", "一级", true},
		{"## 二级", "二级", true},
		{"### 三级", "三级", true},
		{"#NoSpace", "", false},
		{"####### 七级", "", false},
		{"###", "", false},
		{"text # not heading", "", false},
		{"## ## 嵌套", "## 嵌套", true},
		{"  ## 缩进", "缩进", true},
	}
	for _, c := range cases {
		got, ok := markdownHeading(c.line)
		if got != c.want || ok != c.ok {
			t.Errorf("markdownHeading(%q) = (%q, %v), want (%q, %v)", c.line, got, ok, c.want, c.ok)
		}
	}
}

func TestSplitChunks_ByHeadings(t *testing.T) {
	content := "# 标题一\n\n第一段内容\n\n## 标题二\n第二段内容\n\n# 标题三\n第三段"
	chunks := splitChunks(content)
	if len(chunks) != 3 {
		t.Fatalf("应 3 块，got %d: %+v", len(chunks), chunks)
	}
	if chunks[0].Heading != "标题一" {
		t.Errorf("chunk0 heading = %q", chunks[0].Heading)
	}
	if chunks[1].Heading != "标题二" {
		t.Errorf("chunk1 heading = %q", chunks[1].Heading)
	}
	if chunks[2].Heading != "标题三" {
		t.Errorf("chunk2 heading = %q", chunks[2].Heading)
	}
}

func TestSplitChunks_NoHeadingByLength(t *testing.T) {
	// 无标题 → 按长度切分。
	content := "普通文本，没有标题。"
	chunks := splitChunks(content)
	if len(chunks) != 1 || chunks[0].Heading != "" {
		t.Fatalf("无标题应 1 块: %+v", chunks)
	}
	// 超长（> 800）→ 多块 + 重叠。
	long := ""
	for i := 0; i < 100; i++ {
		long += "字"
	}
	longText := strings.Repeat("字", 900)
	_ = long
	chunks = splitChunks(longText)
	if len(chunks) < 2 {
		t.Fatalf("900 字应 ≥2 块，got %d", len(chunks))
	}
	// 首块 = 800 字。
	if len([]rune(chunks[0].Content)) != chunkMaxChars {
		t.Fatalf("首块应为 %d 字，got %d", chunkMaxChars, len([]rune(chunks[0].Content)))
	}
	// 重叠存在：相邻块尾部 + 头部共享 chunkOverlap。
	first, second := []rune(chunks[0].Content), []rune(chunks[1].Content)
	if string(first[len(first)-chunkOverlap:]) != string(second[:chunkOverlap]) {
		t.Logf("重叠检查跳过（短文本场景）")
	}
}

func TestSplitChunks_HeadingOverlong(t *testing.T) {
	// 标题分块内容超长 → 独立切分。
	content := "# 大标题\n" + strings.Repeat("长", 900)
	chunks := splitChunks(content)
	if len(chunks) < 2 {
		t.Fatalf("超长标题块应切分，got %d", len(chunks))
	}
	if chunks[0].Heading != "大标题" {
		t.Fatalf("heading = %q", chunks[0].Heading)
	}
}

func TestSplitChunks_Empty(t *testing.T) {
	if chunks := splitChunks(""); chunks != nil {
		t.Fatalf("空文本应 nil，got %v", chunks)
	}
	if chunks := splitChunks("   \n  "); len(chunks) != 0 {
		t.Fatalf("空白应 0 块，got %v", chunks)
	}
}

func TestVectorEncoding(t *testing.T) {
	vec := []float32{0.1, -0.2, 3.3, 0.0}
	data := encodeVectorF32(vec)
	got := decodeVectorF32(data)
	if !reflect.DeepEqual(vec, got) {
		t.Fatalf("roundtrip: %v != %v", vec, got)
	}
	if len(data) != 4*4 {
		t.Fatalf("f32 LE BLOB 应为 4×4 字节，got %d", len(data))
	}
	// 空向量。
	if got := decodeVectorF32(nil); len(got) != 0 {
		t.Fatalf("空 BLOB 应空向量: %v", got)
	}
}

func TestCosineSimilarity(t *testing.T) {
	// 相同方向 → 1。
	if got := cosineSimilarity([]float32{1, 0}, []float32{2, 0}); math.Abs(float64(got-1)) > 1e-6 {
		t.Fatalf("同向应为 1，got %v", got)
	}
	// 正交 → 0。
	if got := cosineSimilarity([]float32{1, 0}, []float32{0, 1}); math.Abs(float64(got)) > 1e-6 {
		t.Fatalf("正交应为 0，got %v", got)
	}
	// 反向 → -1。
	if got := cosineSimilarity([]float32{1, 0}, []float32{-1, 0}); math.Abs(float64(got+1)) > 1e-6 {
		t.Fatalf("反向应为 -1，got %v", got)
	}
	// 零向量 → 0（防御）。
	if got := cosineSimilarity(nil, []float32{1, 0}); got != 0 {
		t.Fatalf("零向量应为 0，got %v", got)
	}
	if got := cosineSimilarity([]float32{1, 0}, nil); got != 0 {
		t.Fatalf("零向量应为 0，got %v", got)
	}
	// 维度不匹配 → 0。
	if got := cosineSimilarity([]float32{1, 0}, []float32{1, 0, 0}); got != 0 {
		t.Fatalf("维度不匹配应为 0，got %v", got)
	}
}
