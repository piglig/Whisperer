// Package agent wraps the Anthropic SDK behind an LLM interface and provides
// a GM-style tool-use loop. It is intentionally ignorant of which tools exist;
// callers (orchestrator) supply tool definitions and a dispatch function.
package agent

import (
	"context"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Models 集中放模型常量，便于一处升级。
//
// SDK 当前最新提供 Sonnet 4.5 / Haiku 4.5。Sonnet 4.6 / 4.7 待 SDK 升级后切换。
const (
	ModelGM     = anthropic.ModelClaudeSonnet4_5_20250929
	ModelHelper = anthropic.ModelClaudeHaiku4_5
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
	OpenRouterModelGM     anthropic.Model = "anthropic/claude-4.6-sonnet-20260217"
	OpenRouterModelHelper anthropic.Model = "anthropic/claude-4.5-haiku-20251001"
)

// AnthropicBaseURL 是 Anthropic 官方端点。显式指定 base 可以阻止 SDK 读取
// `ANTHROPIC_BASE_URL` 环境变量——这在用户机器上同时设了 OpenRouter base 时
// 很关键（否则官方模式会把请求发到 OpenRouter，但带 x-api-key 头，401）。
const AnthropicBaseURL = "https://api.anthropic.com"

// LLM 是 Whisperer 用到的 Anthropic 子集。引入 interface 是为了让测试用 fake 替换。
type LLM interface {
	NewMessage(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error)
}

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

// NewMessage 直接转发到 client.Messages.New。
func (a *Anthropic) NewMessage(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	return a.client.Messages.New(ctx, params)
}

// OpenRouterModel 把 Anthropic 官方模型常量加上 "anthropic/" 前缀，得到
// OpenRouter 接受的模型 id。
//
// OpenRouter 路由要求 "<vendor>/<model>" 形式；同一模型名（如 claude-sonnet-4-5）
// 在 anthropic 名下与 openai-compat 名下含义不同。
func OpenRouterModel(m anthropic.Model) anthropic.Model {
	if m == "" {
		return m
	}
	s := string(m)
	if len(s) >= len("anthropic/") && s[:len("anthropic/")] == "anthropic/" {
		return m
	}
	return anthropic.Model("anthropic/" + s)
}
