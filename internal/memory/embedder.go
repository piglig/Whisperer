// Package memory wraps chromem-go and exposes a small typed API for the GM
// orchestrator: upsert events / NPC profiles / clues into one of three
// collections, query top-K by cosine similarity, persist to disk per save.
//
// 测试不依赖外部 embedding 服务：FakeEmbedder 用 hash → 稀疏向量，输出确定性。
// 生产用真实 embedder：通过 NewEmbedder 工厂选择 OpenAI / Cohere / Ollama / Local
// 或显式 OpenAI-Compatible（包括 OpenRouter 等）。
package memory

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	chromem "github.com/philippgille/chromem-go"
)

// Embedder 是 chromem-go 的 EmbeddingFunc 别名，便于在 whisperer 内统一签名。
type Embedder = chromem.EmbeddingFunc

// EmbedderConfig 选择并构造一个生产级 embedder。零值 → fake。
//
// Provider 取值（与 CLI --embedder 对齐）：
//   - "fake" / ""     —— 确定性 hash embedder，无外部调用，仅用于测试
//   - "openai"        —— OpenAI 官方；需要 APIKey；Model 默认 text-embedding-3-small
//   - "openai-compat" —— OpenAI 兼容端点（OpenRouter、本地 LiteLLM 网关等）；
//     必须显式 BaseURL；Model 必须显式
//   - "cohere"        —— Cohere 官方；需要 APIKey；Model 默认 embed-multilingual-v3.0
//   - "ollama"        —— 本地 Ollama；BaseURL 默认 http://localhost:11434/api；
//     Model 必须显式（如 "nomic-embed-text"）
//   - "localai"       —— 本地 LocalAI；Model 必须显式
type EmbedderConfig struct {
	Provider string
	APIKey   string
	Model    string
	BaseURL  string
}

// NewEmbedder 按 cfg.Provider 构造 embedder。详见 EmbedderConfig 文档。
func NewEmbedder(cfg EmbedderConfig) (Embedder, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "", "fake":
		return NewFakeEmbedder(0), nil
	case "openai":
		if cfg.APIKey == "" {
			return nil, errors.New("embedder openai: api key required")
		}
		model := cfg.Model
		if model == "" {
			model = string(chromem.EmbeddingModelOpenAI3Small)
		}
		return chromem.NewEmbeddingFuncOpenAI(cfg.APIKey, chromem.EmbeddingModelOpenAI(model)), nil
	case "openai-compat":
		if cfg.BaseURL == "" {
			return nil, errors.New("embedder openai-compat: base_url required")
		}
		if cfg.Model == "" {
			return nil, errors.New("embedder openai-compat: model required")
		}
		return chromem.NewEmbeddingFuncOpenAICompat(cfg.BaseURL, cfg.APIKey, cfg.Model, nil), nil
	case "cohere":
		if cfg.APIKey == "" {
			return nil, errors.New("embedder cohere: api key required")
		}
		model := cfg.Model
		if model == "" {
			model = string(chromem.EmbeddingModelCohereMultilingualV3)
		}
		return chromem.NewEmbeddingFuncCohere(cfg.APIKey, chromem.EmbeddingModelCohere(model)), nil
	case "ollama":
		if cfg.Model == "" {
			return nil, errors.New("embedder ollama: model required (e.g. nomic-embed-text)")
		}
		base := cfg.BaseURL
		if base == "" {
			base = "http://localhost:11434/api"
		}
		return chromem.NewEmbeddingFuncOllama(cfg.Model, base), nil
	case "localai":
		if cfg.Model == "" {
			return nil, errors.New("embedder localai: model required")
		}
		return chromem.NewEmbeddingFuncLocalAI(cfg.Model), nil
	default:
		return nil, fmt.Errorf("embedder: unknown provider %q (want fake|openai|openai-compat|cohere|ollama|localai)", cfg.Provider)
	}
}

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
