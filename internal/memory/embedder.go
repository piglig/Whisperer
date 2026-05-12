// Package memory wraps chromem-go and exposes a small typed API for the GM
// orchestrator: upsert events / NPC profiles / clues into one of three
// collections, query top-K by cosine similarity, persist to disk per save.
//
// 测试不依赖外部 embedding 服务：FakeEmbedder 用 hash → 稀疏向量，输出确定性。
// 生产替换为 chromem-go 自带的 OpenAI / Voyage / Ollama 工厂。
package memory

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strings"

	chromem "github.com/philippgille/chromem-go"
)

// Embedder 是 chromem-go 的 EmbeddingFunc 别名，便于在 whisperer 内统一签名。
type Embedder = chromem.EmbeddingFunc

// FakeEmbedderDim 是 NewFakeEmbedder 默认输出维度。低维度足够测试用，
// 同时让相似度差异容易在断言中放大。
const FakeEmbedderDim = 32

// NewFakeEmbedder 返回一个确定性、无外部依赖的 embedder：
//   - 对输入文本切 token（按非字母数字切分）
//   - 对每个 token 算 sha256，按 (hash mod dim) 累加权重
//   - L2 归一化
//
// 同 dim 的两段相似文本（共享更多 token）会得到更高的余弦相似度，足够覆盖
// memory_test.go 里的相对排序断言。不要用于生产。
func NewFakeEmbedder(dim int) Embedder {
	if dim <= 0 {
		dim = FakeEmbedderDim
	}
	return func(_ context.Context, text string) ([]float32, error) {
		if strings.TrimSpace(text) == "" {
			return nil, errors.New("memory.FakeEmbedder: empty input")
		}
		vec := make([]float32, dim)
		for _, tok := range tokenize(text) {
			h := sha256.Sum256([]byte(tok))
			// 用前 8 字节做 bucket，后 8 字节做权重符号。
			bucket := int(binary.BigEndian.Uint64(h[:8]) % uint64(dim))
			weight := float32(1.0)
			if h[15]&1 == 1 {
				weight = -1.0
			}
			vec[bucket] += weight
		}
		var norm float32
		for _, v := range vec {
			norm += v * v
		}
		if norm == 0 {
			// 极端情况下（全部 token 抵消）退化为 token 的位置 buckets。
			vec[0] = 1
			return vec, nil
		}
		// L2 normalize
		inv := 1.0 / float32Sqrt(norm)
		for i := range vec {
			vec[i] *= inv
		}
		return vec, nil
	}
}

func tokenize(text string) []string {
	var tokens []string
	var cur strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			cur.WriteRune(r)
		default:
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// 引入一个独立的 sqrt 让我们避免直接依赖 math 包，以便单测对比 chromem-go
// 内部归一化的小差异。实际就是 math.Sqrt 的 float32 包装。
func float32Sqrt(x float32) float32 {
	// Newton-Raphson，4 步精度对 32 维归一化够用。
	if x == 0 {
		return 0
	}
	z := x
	for range 6 {
		z = (z + x/z) / 2
	}
	return z
}
