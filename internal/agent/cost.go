package agent

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// ModelPricing 是某模型的每百万 token 价格（USD）。
//
// 数据来源：Anthropic 官方价格页（截至 2026-05）。OpenRouter 转售价格可能高出
// 5–10%，本表只用作"近似演示用 cost 估算"；如果用户启用 OpenRouter，最终账单以
// OpenRouter 后台为准。
type ModelPricing struct {
	InputPerMTok  float64 // 输入 token 单价
	OutputPerMTok float64 // 输出 token 单价
}

// pricingTable 把模型名前缀映射到价格。匹配按"最长前缀优先"避免 4_5 误匹配 4。
//
// 升级新模型时只在此表加一条；其他代码无需改动。
var pricingTable = []struct {
	prefix  string
	pricing ModelPricing
}{
	// Anthropic 官方
	{"claude-opus-4-7", ModelPricing{InputPerMTok: 15.0, OutputPerMTok: 75.0}},
	{"claude-opus-4-5", ModelPricing{InputPerMTok: 15.0, OutputPerMTok: 75.0}},
	{"claude-opus-4", ModelPricing{InputPerMTok: 15.0, OutputPerMTok: 75.0}},
	{"claude-sonnet-4-7", ModelPricing{InputPerMTok: 3.0, OutputPerMTok: 15.0}},
	{"claude-sonnet-4-6", ModelPricing{InputPerMTok: 3.0, OutputPerMTok: 15.0}},
	{"claude-sonnet-4-5", ModelPricing{InputPerMTok: 3.0, OutputPerMTok: 15.0}},
	{"claude-sonnet-4", ModelPricing{InputPerMTok: 3.0, OutputPerMTok: 15.0}},
	{"claude-haiku-4-5", ModelPricing{InputPerMTok: 1.0, OutputPerMTok: 5.0}},
	{"claude-haiku-4", ModelPricing{InputPerMTok: 1.0, OutputPerMTok: 5.0}},
	{"claude-3-5-sonnet", ModelPricing{InputPerMTok: 3.0, OutputPerMTok: 15.0}},
	{"claude-3-5-haiku", ModelPricing{InputPerMTok: 0.80, OutputPerMTok: 4.0}},

	// OpenRouter 命名（"anthropic/claude-..."）—— 与 Anthropic 同价的近似估算
	{"anthropic/claude-opus-4", ModelPricing{InputPerMTok: 15.0, OutputPerMTok: 75.0}},
	{"anthropic/claude-sonnet-4", ModelPricing{InputPerMTok: 3.0, OutputPerMTok: 15.0}},
	{"anthropic/claude-4.6-sonnet", ModelPricing{InputPerMTok: 3.0, OutputPerMTok: 15.0}},
	{"anthropic/claude-4.5-sonnet", ModelPricing{InputPerMTok: 3.0, OutputPerMTok: 15.0}},
	{"anthropic/claude-4.5-haiku", ModelPricing{InputPerMTok: 1.0, OutputPerMTok: 5.0}},
	{"anthropic/claude-haiku-4", ModelPricing{InputPerMTok: 1.0, OutputPerMTok: 5.0}},
	{"anthropic/claude-3-5-sonnet", ModelPricing{InputPerMTok: 3.0, OutputPerMTok: 15.0}},
	{"anthropic/claude-3-5-haiku", ModelPricing{InputPerMTok: 0.80, OutputPerMTok: 4.0}},
}

// PricingFor 按模型名查价。未知模型返回零值（cost 计算结果为 0，不阻塞流程）。
//
// 匹配按"最长前缀优先"：例如 "claude-sonnet-4-5-20250929" 命中 "claude-sonnet-4-5"
// 而不会被更短的 "claude-sonnet-4" 抢占。
func PricingFor(model anthropic.Model) ModelPricing {
	name := strings.ToLower(string(model))
	bestLen := -1
	var best ModelPricing
	for _, row := range pricingTable {
		p := strings.ToLower(row.prefix)
		if strings.HasPrefix(name, p) && len(p) > bestLen {
			bestLen = len(p)
			best = row.pricing
		}
	}
	return best
}

// CostUSD 按价格表计算单次调用的费用。tokens 为零或模型未知时返回 0。
func CostUSD(model anthropic.Model, inputTokens, outputTokens int64) (inputUSD, outputUSD, totalUSD float64) {
	p := PricingFor(model)
	const million = 1_000_000.0
	inputUSD = float64(inputTokens) / million * p.InputPerMTok
	outputUSD = float64(outputTokens) / million * p.OutputPerMTok
	totalUSD = inputUSD + outputUSD
	return inputUSD, outputUSD, totalUSD
}
