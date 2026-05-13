package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// DefaultMaxIterations 是单次 Respond 内允许的 LLM ↔ tool 回合数上限。
//
// 超出即停止并把 TurnTrace.Truncated 置真。orchestrator 之后可据此降级处理。
const DefaultMaxIterations = 10

// DefaultMaxTokens 控制单次 Messages.New 的最大输出 token。
const DefaultMaxTokens int64 = 4096

// ToolHandler 是 dispatcher 暴露给 GM agent 的统一回调签名。
//
// 输入：tool 名字 + LLM 提供的原始 input JSON。
// 输出：handler 自己的结果（任意可序列化结构），以及 isError 标志。
// handler 内部错误一律通过返回 (errStruct, true) 表达，不要返回 Go error；
// 这样 LLM 能看到错误内容并做出调整，而不是中断整个回合。
type ToolHandler func(ctx context.Context, name string, inputRaw json.RawMessage) (output any, isError bool)

// GMAgent 是封装好的 GM tool-use 主循环。
type GMAgent struct {
	llm       LLM
	model     anthropic.Model
	system    []anthropic.TextBlockParam
	tools     []anthropic.ToolUnionParam
	handler   ToolHandler
	maxIter   int
	maxTokens int64
}

// Config 构造 GMAgent。零值字段会被填默认值。
type Config struct {
	LLM           LLM
	Model         anthropic.Model
	SystemPrompt  string // 普通字符串；内部包装为单个 TextBlockParam（带 cache_control）
	Tools         []anthropic.ToolUnionParam
	Handler       ToolHandler
	MaxIterations int
	MaxTokens     int64
}

// New 从 Config 构造 GMAgent。
func New(cfg Config) (*GMAgent, error) {
	if cfg.LLM == nil {
		return nil, errors.New("agent.New: LLM is required")
	}
	if cfg.Handler == nil {
		return nil, errors.New("agent.New: Handler is required")
	}
	if cfg.Model == "" {
		cfg.Model = ModelGM
	}
	if cfg.MaxIterations == 0 {
		cfg.MaxIterations = DefaultMaxIterations
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = DefaultMaxTokens
	}

	system := []anthropic.TextBlockParam{}
	if strings.TrimSpace(cfg.SystemPrompt) != "" {
		// 给 system prompt 标 cache_control=ephemeral，单回合多次 LLM 调用复用缓存。
		system = append(system, anthropic.TextBlockParam{
			Text:         cfg.SystemPrompt,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		})
	}

	return &GMAgent{
		llm:       cfg.LLM,
		model:     cfg.Model,
		system:    system,
		tools:     cfg.Tools,
		handler:   cfg.Handler,
		maxIter:   cfg.MaxIterations,
		maxTokens: cfg.MaxTokens,
	}, nil
}

// Respond 在给定历史与新输入上跑一次完整的 tool_use 循环。
//
// 返回:
//   - TurnTrace 记录文本与工具调用
//   - 更新后的 messages，调用方可保留作为下一回合 history
//   - error 仅在 LLM 调用失败或 handler 返回完全无法序列化的结果时
func (g *GMAgent) Respond(
	ctx context.Context,
	history []anthropic.MessageParam,
	userInput string,
) (TurnTrace, []anthropic.MessageParam, error) {
	messages := append([]anthropic.MessageParam(nil), history...)
	messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(userInput)))

	trace := TurnTrace{}
	var narrativeBuilder strings.Builder

	for iter := 1; iter <= g.maxIter; iter++ {
		trace.Iterations = iter

		params := anthropic.MessageNewParams{
			Model:     g.model,
			MaxTokens: g.maxTokens,
			System:    g.system,
			Messages:  messages,
			Tools:     g.tools,
		}
		msg, err := g.llm.NewMessage(ctx, params)
		if err != nil {
			return trace, messages, fmt.Errorf("agent: LLM call failed at iter %d: %w", iter, err)
		}
		trace.InputTokens += msg.Usage.InputTokens
		trace.OutputTokens += msg.Usage.OutputTokens
		// 累加 cost：每次 LLM 调用都用 g.model 当时的价格表查一次。
		// 单回合通常用同一模型，但理论上未来可能在 iter 之间切换；分摊到每次调用更稳。
		inUSD, outUSD, _ := CostUSD(g.model, msg.Usage.InputTokens, msg.Usage.OutputTokens)
		trace.InputCostUSD += inUSD
		trace.OutputCostUSD += outUSD
		trace.TotalCostUSD = trace.InputCostUSD + trace.OutputCostUSD

		// 把本轮 assistant 输出加进对话。
		messages = append(messages, msg.ToParam())

		var toolResults []anthropic.ContentBlockParamUnion

		for _, block := range msg.Content {
			switch b := block.AsAny().(type) {
			case anthropic.TextBlock:
				if narrativeBuilder.Len() > 0 {
					narrativeBuilder.WriteString("\n")
				}
				narrativeBuilder.WriteString(b.Text)

			case anthropic.ToolUseBlock:
				out, isErr := g.handler(ctx, b.Name, b.Input)
				outBytes, mErr := json.Marshal(out)
				if mErr != nil {
					// handler 输出无法序列化 → 视为工具错误，让 LLM 看到。
					outBytes = []byte(fmt.Sprintf(`{"error":"output marshal: %s"}`, mErr.Error()))
					isErr = true
				}

				trace.ToolCalls = append(trace.ToolCalls, ToolCall{
					ID:      b.ID,
					Name:    b.Name,
					Input:   append(json.RawMessage(nil), b.Input...),
					Output:  append(json.RawMessage(nil), outBytes...),
					IsError: isErr,
					Iter:    iter,
				})

				toolResults = append(toolResults, anthropic.NewToolResultBlock(b.ID, string(outBytes), isErr))
			}
		}

		if len(toolResults) == 0 {
			// 没有工具调用 → 本回合结束。
			trace.Narrative = narrativeBuilder.String()
			return trace, messages, nil
		}

		messages = append(messages, anthropic.NewUserMessage(toolResults...))
	}

	// 用尽迭代仍未结束。
	trace.Truncated = true
	trace.Narrative = narrativeBuilder.String()
	return trace, messages, nil
}
