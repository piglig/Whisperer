// Package agent wraps LLM providers behind a provider-neutral interface and provides
// a GM-style tool-use loop. It is intentionally ignorant of which tools exist;
// callers (orchestrator) supply tool definitions and a dispatch function.
package agent

import (
	"context"
	"encoding/json"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Models 集中放模型常量，便于一处升级。
//
// SDK 当前最新提供 Sonnet 4.5 / Haiku 4.5。Sonnet 4.6 / 4.7 待 SDK 升级后切换。
const (
	ModelGM     Model = Model(anthropic.ModelClaudeSonnet4_5_20250929)
	ModelHelper Model = Model(anthropic.ModelClaudeHaiku4_5)
)

// OpenRouterBaseURL 是 OpenRouter 的 Anthropic-兼容端点根路径。
//
// 注意 SDK 会自动追加 "/v1/messages"，所以 base 写到 "/api" 即可（与 Anthropic
// 默认 base "https://api.anthropic.com" 同语义）。
const OpenRouterBaseURL = "https://openrouter.ai/api"

// OpenRouter 默认模型常量。
//
// 选这两个版本的原因：常见 OpenRouter 企业账户的数据策略 / 白名单倾向于允许较
// 新的 Anthropic 主推版本（4.6 sonnet 与 4.5 haiku）。如果你的账户白名单不同，
// 可以用 cmd 的 --model / --model-helper flag 覆盖。
const (
	OpenRouterModelGM     Model = "anthropic/claude-4.6-sonnet-20260217"
	OpenRouterModelHelper Model = "anthropic/claude-4.5-haiku-20251001"
)

// AnthropicBaseURL 是 Anthropic 官方端点。显式指定 base 可以阻止 SDK 读取
// `ANTHROPIC_BASE_URL` 环境变量——这在用户机器上同时设了 OpenRouter base 时
// 很关键（否则官方模式会把请求发到 OpenRouter，但带 x-api-key 头，401）。
const AnthropicBaseURL = "https://api.anthropic.com"

// Anthropic 包装真实的 anthropic.Client，使其满足 LLM。
type Anthropic struct {
	client anthropic.Client
}

// ClientConfig 收敛构造 Anthropic 客户端的可调项。
//
//   - APIKey 走 x-api-key header（Anthropic 官方）
//   - AuthToken 走 Authorization: Bearer header（OpenRouter 等代理用）
//   - 两者择一；APIKey 与 AuthToken 同时给以 AuthToken 为准
//   - BaseURL 缺省走 SDK 默认（生产 Anthropic 端点）
//   - MaxRetries 走 SDK 内置指数退避；0 表示用 SDK 默认（当前为 2）
//   - RequestTimeout 是单次 LLM 调用的硬超时；0 表示不设
type ClientConfig struct {
	APIKey         string
	AuthToken      string
	BaseURL        string
	MaxRetries     int
	RequestTimeout time.Duration
}

// NewAnthropic 用 ClientConfig 构造客户端。
//
// 调用方典型用法：
//
//	llm := agent.NewAnthropic(agent.ClientConfig{APIKey: os.Getenv("ANTHROPIC_API_KEY")})
//
//	// OpenRouter
//	llm := agent.NewAnthropic(agent.ClientConfig{
//	    AuthToken: os.Getenv("OPENROUTER_API_KEY"),
//	    BaseURL:   agent.OpenRouterBaseURL,
//	})
func NewAnthropic(cfg ClientConfig) *Anthropic {
	var opts []option.RequestOption
	if cfg.AuthToken != "" {
		opts = append(opts, option.WithAuthToken(cfg.AuthToken))
	} else if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	if cfg.MaxRetries > 0 {
		opts = append(opts, option.WithMaxRetries(cfg.MaxRetries))
	}
	if cfg.RequestTimeout > 0 {
		opts = append(opts, option.WithRequestTimeout(cfg.RequestTimeout))
	}
	return &Anthropic{client: anthropic.NewClient(opts...)}
}

func (a *Anthropic) NewMessage(ctx context.Context, params MessageRequest) (*Message, error) {
	resp, err := a.client.Messages.New(ctx, toAnthropicMessageParams(params))
	if err != nil {
		return nil, err
	}
	return messageFromAnthropic(resp), nil
}

// OpenRouterModel 把 Anthropic 官方模型常量加上 "anthropic/" 前缀，得到
// OpenRouter 接受的模型 id。
//
// OpenRouter 路由要求 "<vendor>/<model>" 形式；同一模型名（如 claude-sonnet-4-5）
// 在 anthropic 名下与 openai-compat 名下含义不同。
func OpenRouterModel(m Model) Model {
	if m == "" {
		return m
	}
	s := string(m)
	if len(s) >= len("anthropic/") && s[:len("anthropic/")] == "anthropic/" {
		return m
	}
	return Model("anthropic/" + s)
}

func toAnthropicMessageParams(params MessageRequest) anthropic.MessageNewParams {
	out := anthropic.MessageNewParams{
		Model:     anthropic.Model(params.Model),
		MaxTokens: params.MaxTokens,
		System:    make([]anthropic.TextBlockParam, 0, len(params.System)),
		Messages:  make([]anthropic.MessageParam, 0, len(params.Messages)),
		Tools:     make([]anthropic.ToolUnionParam, 0, len(params.Tools)),
	}
	for _, block := range params.System {
		tb := anthropic.TextBlockParam{Text: block.Text}
		if block.CacheEphemeral {
			tb.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		out.System = append(out.System, tb)
	}
	for _, msg := range params.Messages {
		out.Messages = append(out.Messages, messageParamToAnthropic(msg))
	}
	for _, tool := range params.Tools {
		t := anthropic.ToolParam{
			Name:        tool.Name,
			Description: anthropic.String(tool.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: tool.InputSchema.Properties,
				Required:   tool.InputSchema.Required,
			},
		}
		out.Tools = append(out.Tools, anthropic.ToolUnionParam{OfTool: &t})
	}
	return out
}

func messageParamToAnthropic(msg MessageParam) anthropic.MessageParam {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(msg.Content))
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			blocks = append(blocks, anthropic.NewTextBlock(block.Text))
		case "tool_use":
			var input any = map[string]any{}
			if len(block.Input) > 0 {
				var decoded any
				if err := json.Unmarshal(block.Input, &decoded); err == nil {
					input = decoded
				} else {
					input = json.RawMessage(block.Input)
				}
			}
			blocks = append(blocks, anthropic.NewToolUseBlock(block.ID, input, block.Name))
		case "tool_result":
			blocks = append(blocks, anthropic.NewToolResultBlock(block.ToolUseID, block.Text, block.IsError))
		}
	}
	if msg.Role == RoleAssistant {
		return anthropic.NewAssistantMessage(blocks...)
	}
	return anthropic.NewUserMessage(blocks...)
}

func messageFromAnthropic(msg *anthropic.Message) *Message {
	if msg == nil {
		return nil
	}
	out := &Message{
		ID:         msg.ID,
		Role:       Role(msg.Role),
		Model:      Model(msg.Model),
		StopReason: StopReason(msg.StopReason),
		Content:    make([]ContentBlock, 0, len(msg.Content)),
		Usage: Usage{
			InputTokens:  msg.Usage.InputTokens,
			OutputTokens: msg.Usage.OutputTokens,
		},
	}
	for _, block := range msg.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			out.Content = append(out.Content, ContentBlock{Type: "text", Text: b.Text})
		case anthropic.ToolUseBlock:
			input, _ := json.Marshal(b.Input)
			if len(input) == 0 {
				input = []byte("{}")
			}
			out.Content = append(out.Content, ContentBlock{
				Type:  "tool_use",
				ID:    b.ID,
				Name:  b.Name,
				Input: json.RawMessage(input),
			})
		}
	}
	return out
}
