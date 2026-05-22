package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// tracerName 与 internal/telemetry.TracerName 保持一致——避免 import cycle 的同时
// 让所有 whisperer 包的 span 都聚合到同一个 instrumentation scope。
const tracerName = "github.com/zhuzhenwu/whisperer"

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
	model     Model
	system    []SystemBlock
	tools     []ToolDefinition
	handler   ToolHandler
	maxIter   int
	maxTokens int64
}

// Config 构造 GMAgent。零值字段会被填默认值。
type Config struct {
	LLM           LLM
	Model         Model
	SystemPrompt  string // 普通字符串；内部包装为单个 TextBlockParam（带 cache_control）
	Tools         []ToolDefinition
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

	system := []SystemBlock{}
	if strings.TrimSpace(cfg.SystemPrompt) != "" {
		// 给 system prompt 标 cache_control=ephemeral，单回合多次 LLM 调用复用缓存。
		system = append(system, SystemBlock{Text: cfg.SystemPrompt, CacheEphemeral: true})
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
	history []MessageParam,
	userInput string,
) (TurnTrace, []MessageParam, error) {
	messages := append([]MessageParam(nil), history...)
	messages = append(messages, NewUserMessage(NewTextBlock(userInput)))

	trace := TurnTrace{}
	var narrativeBuilder strings.Builder

	for iter := 1; iter <= g.maxIter; iter++ {
		trace.Iterations = iter

		params := MessageRequest{
			Model:     g.model,
			MaxTokens: g.maxTokens,
			System:    g.system,
			Messages:  messages,
			Tools:     g.tools,
		}
		callCtx, callSpan := otel.Tracer(tracerName).Start(ctx, "whisperer.llm.message")
		callSpan.SetAttributes(
			attribute.String("llm.model", string(g.model)),
			attribute.Int("llm.iter", iter),
		)
		msg, err := g.llm.NewMessage(callCtx, params)
		if err != nil {
			callSpan.SetStatus(codes.Error, "LLM call failed")
			callSpan.RecordError(err)
			callSpan.End()
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
		callSpan.SetAttributes(
			attribute.Int64("llm.input_tokens", msg.Usage.InputTokens),
			attribute.Int64("llm.output_tokens", msg.Usage.OutputTokens),
			attribute.Float64("llm.cost_usd", inUSD+outUSD),
			attribute.String("llm.stop_reason", string(msg.StopReason)),
		)
		callSpan.End()

		// 把本轮 assistant 输出加进对话。
		messages = append(messages, msg.ToParam())

		var toolResults []ContentBlockParam

		for _, block := range msg.Content {
			switch block.Type {
			case "text":
				if narrativeBuilder.Len() > 0 {
					narrativeBuilder.WriteString("\n")
				}
				narrativeBuilder.WriteString(block.Text)

			case "tool_use":
				out, isErr := g.handler(ctx, block.Name, block.Input)
				outBytes, mErr := json.Marshal(out)
				if mErr != nil {
					// handler 输出无法序列化 → 视为工具错误，让 LLM 看到。
					outBytes = []byte(fmt.Sprintf(`{"error":"output marshal: %s"}`, mErr.Error()))
					isErr = true
				}

				trace.ToolCalls = append(trace.ToolCalls, ToolCall{
					ID:      block.ID,
					Name:    block.Name,
					Input:   append(json.RawMessage(nil), block.Input...),
					Output:  append(json.RawMessage(nil), outBytes...),
					IsError: isErr,
					Iter:    iter,
				})

				toolResults = append(toolResults, NewToolResultBlock(block.ID, string(outBytes), isErr))
			}
		}

		if len(toolResults) == 0 {
			// 没有工具调用 → 本回合结束。
			trace.Narrative = narrativeBuilder.String()
			return trace, messages, nil
		}

		messages = append(messages, NewUserMessage(toolResults...))
	}

	// 用尽迭代仍未结束。
	trace.Truncated = true
	trace.Narrative = narrativeBuilder.String()
	return trace, messages, nil
}
