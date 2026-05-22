package agent

import (
	"context"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	vcr "gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

// TestCassette_ReplaySmoke 验证 cassette 录放整链路：
//   - Recorder 在 ModeReplayOnly 下从 testdata 读 fixture
//   - Anthropic SDK 经由 recorder 的 http.Client 发送请求
//   - SDK 把 fixture 响应解析为 *agent.Message
//   - cost 计算可以挂在结果上
//
// 这个测试 **不需要任何 API key**——它从手工录入的 fixture 重放。
func TestCassette_ReplaySmoke(t *testing.T) {
	mode := vcr.ModeReplayOnly
	rec, err := NewCassetteRecorder(CassetteOptions{
		Path: "testdata/cassettes/replay_smoke",
		Mode: &mode,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = rec.Stop() })

	require.Equal(t, vcr.ModeReplayOnly, rec.Mode())

	client := anthropic.NewClient(
		option.WithAPIKey("test-key-not-used"),
		// 显式钉死 BaseURL，避免开发机上的 ANTHROPIC_BASE_URL 把请求改路
		option.WithBaseURL(AnthropicBaseURL),
		option.WithHTTPClient(rec.Client()),
	)

	msg, err := client.Messages.New(context.Background(), anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-haiku-4-5"),
		MaxTokens: 256,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("replay smoke")),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, msg)

	require.Len(t, msg.Content, 1)
	if tb, ok := msg.Content[0].AsAny().(anthropic.TextBlock); ok {
		assert.Equal(t, "replay ok", tb.Text)
	} else {
		t.Fatalf("expected TextBlock, got %T", msg.Content[0].AsAny())
	}

	// 验证 cost 计算能正常吃 fixture 的 usage 字段
	_, _, total := CostUSD(Model("claude-haiku-4-5"),
		msg.Usage.InputTokens, msg.Usage.OutputTokens)
	assert.Greater(t, total, 0.0, "haiku pricing should produce non-zero cost from fixture usage")
}

func TestCassette_DefaultsToReplay(t *testing.T) {
	t.Setenv("WHISPERER_VCR_RECORD", "")
	rec, err := NewCassetteRecorder(CassetteOptions{
		Path: "testdata/cassettes/replay_smoke",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = rec.Stop() })
	assert.Equal(t, vcr.ModeReplayOnly, rec.Mode())
}

func TestCassette_RequiresPath(t *testing.T) {
	_, err := NewCassetteRecorder(CassetteOptions{})
	require.Error(t, err)
}
