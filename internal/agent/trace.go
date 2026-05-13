package agent

import "encoding/json"

// ToolCall 记录一次 LLM 触发的 tool 调用，包括 LLM 给的 input 与我方 handler 的 output。
//
// 用作 SLA 校验器（W4-W7）的输入：例如 SLA #1 "检定即工具" 会扫描 ToolCalls 是否包含
// `roll_*` 名字。
type ToolCall struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Input   json.RawMessage `json:"input"`
	Output  json.RawMessage `json:"output"`
	IsError bool            `json:"is_error"`
	Iter    int             `json:"iter"`
}

// TurnTrace 是 GMAgent.Respond 一次调用的完整记录。
//
// Narrative 是所有 TextBlock 的拼接（按出现顺序），便于 SLA #6 "失败即失败" 等基于
// 文本的反向匹配。Iterations 计 LLM 回合数；超过 maxIter 时 Truncated 为真。
//
// Cost 字段是基于 internal/agent/cost.go 价格表的近似估算，模型未知时为 0。
type TurnTrace struct {
	Narrative     string     `json:"narrative"`
	ToolCalls     []ToolCall `json:"tool_calls"`
	Iterations    int        `json:"iterations"`
	Truncated     bool       `json:"truncated"`
	InputTokens   int64      `json:"input_tokens"`
	OutputTokens  int64      `json:"output_tokens"`
	InputCostUSD  float64    `json:"input_cost_usd"`
	OutputCostUSD float64    `json:"output_cost_usd"`
	TotalCostUSD  float64    `json:"total_cost_usd"`
}
