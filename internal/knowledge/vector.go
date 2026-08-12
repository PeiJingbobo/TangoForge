package knowledge

import (
	"encoding/binary"
	"math"
)

// encodeVectorF32 将 f32 切片编码为 little-endian 字节序列（knowledge_chunks.vector BLOB）。
func encodeVectorF32(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// decodeVectorF32 从 little-endian 字节序列解码 f32 切片。
func decodeVectorF32(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}

// cosineSimilarity 纯 Go 余弦相似度（全表线性扫描，QA-K22：<5k 文档性能可接受）。
// 零向量 / 维度不匹配 → 0（防御，不视为错误）。
func cosineSimilarity(a, b []float32) float32 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		fa, fb := float64(a[i]), float64(b[i])
		dot += fa * fb
		na += fa * fa
		nb += fb * fb
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(na) * math.Sqrt(nb)))
}
