package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NewAnthropic 仅做组装，不发请求。这里覆盖：
//   - 各 ClientConfig 排列都能成功 NewClient（不 panic、不返回 nil）
//   - MaxRetries 0 与 RequestTimeout 0 是合法 no-op（走 SDK 默认）
func TestNewAnthropic_AcceptsAllOptions(t *testing.T) {
	cases := []ClientConfig{
		{APIKey: "sk-test"},
		{AuthToken: "or-test", BaseURL: OpenRouterBaseURL},
		{APIKey: "sk-test", MaxRetries: 5},
		{APIKey: "sk-test", RequestTimeout: 30 * time.Second},
		{APIKey: "sk-test", MaxRetries: 2, RequestTimeout: 60 * time.Second},
		{},
	}
	for i, cfg := range cases {
		c := NewAnthropic(cfg)
		require.NotNil(t, c, "case %d", i)
	}
}

func TestOpenRouterModel_AddsPrefixOnce(t *testing.T) {
	assert.Equal(t, "anthropic/claude-sonnet-4-5",
		string(OpenRouterModel("claude-sonnet-4-5")))
	assert.Equal(t, "anthropic/claude-sonnet-4-5",
		string(OpenRouterModel("anthropic/claude-sonnet-4-5")))
	assert.Equal(t, "", string(OpenRouterModel("")))
}
