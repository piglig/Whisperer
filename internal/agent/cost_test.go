package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPricingFor_LongestPrefixWins(t *testing.T) {
	// 4-7 表里有显式条目 → 命中 75 USD/MTok 输出
	p := PricingFor(Model("claude-opus-4-7-20260101"))
	assert.Equal(t, 75.0, p.OutputPerMTok)

	// 4-5 sonnet 应命中 sonnet 行（3/15）而不是 opus 行（15/75）
	p = PricingFor(Model("claude-sonnet-4-5-20250929"))
	assert.Equal(t, 3.0, p.InputPerMTok)
	assert.Equal(t, 15.0, p.OutputPerMTok)

	// 完全不匹配 → 零值
	p = PricingFor(Model("claude-fictional-99"))
	assert.Equal(t, 0.0, p.InputPerMTok)
	assert.Equal(t, 0.0, p.OutputPerMTok)
}

func TestPricingFor_OpenRouterPrefix(t *testing.T) {
	p := PricingFor(Model("anthropic/claude-4.6-sonnet-20260217"))
	assert.Equal(t, 3.0, p.InputPerMTok)
	assert.Equal(t, 15.0, p.OutputPerMTok)
}

func TestCostUSD_Sonnet(t *testing.T) {
	// Sonnet 4.5 价格 3 / 15。10K 输入 + 2K 输出：
	//   input  = 10000 / 1e6 * 3  = 0.030 USD
	//   output = 2000  / 1e6 * 15 = 0.030 USD
	in, out, total := CostUSD(ModelGM, 10_000, 2_000)
	assert.InDelta(t, 0.030, in, 1e-9)
	assert.InDelta(t, 0.030, out, 1e-9)
	assert.InDelta(t, 0.060, total, 1e-9)
}

func TestCostUSD_UnknownModelReturnsZero(t *testing.T) {
	in, out, total := CostUSD(Model("unknown-x"), 999_999, 999_999)
	assert.Equal(t, 0.0, in)
	assert.Equal(t, 0.0, out)
	assert.Equal(t, 0.0, total)
}

func TestCostUSD_Haiku(t *testing.T) {
	// Haiku 4.5 价格 1 / 5。100K 输入 + 5K 输出：
	//   input  = 100000 / 1e6 * 1 = 0.100
	//   output = 5000   / 1e6 * 5 = 0.025
	in, out, total := CostUSD(ModelHelper, 100_000, 5_000)
	assert.InDelta(t, 0.100, in, 1e-9)
	assert.InDelta(t, 0.025, out, 1e-9)
	assert.InDelta(t, 0.125, total, 1e-9)
}
