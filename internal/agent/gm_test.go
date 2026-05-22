package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeLLM 用预排好的脚本回放 *agent.Message。
type fakeLLM struct {
	scripts []string // 每个 string 是一段完整的 *agent.Message JSON
	calls   int
	failOn  int // 第几次调用返回 error；0 表示不失败
	failErr error
}

func (f *fakeLLM) NewMessage(_ context.Context, _ MessageRequest) (*Message, error) {
	idx := f.calls
	f.calls++
	if f.failOn > 0 && f.calls == f.failOn {
		return nil, f.failErr
	}
	if idx >= len(f.scripts) {
		// 默认返回一段空文本结束循环
		return parseMsg(`{"id":"end","role":"assistant","model":"x","stop_reason":"end_turn","type":"message","content":[{"type":"text","text":""}],"usage":{"input_tokens":1,"output_tokens":1}}`), nil
	}
	return parseMsg(f.scripts[idx]), nil
}

func parseMsg(s string) *Message {
	var m Message
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		panic(err)
	}
	return &m
}

// 工具：构造 message JSON 中的 content 块。
func textBlk(text string) string {
	b, _ := json.Marshal(map[string]any{"type": "text", "text": text})
	return string(b)
}

func toolUseBlk(id, name string, input map[string]any) string {
	b, _ := json.Marshal(map[string]any{
		"type":  "tool_use",
		"id":    id,
		"name":  name,
		"input": input,
	})
	return string(b)
}

func msgWith(content ...string) string {
	c := "[" + joinCSV(content) + "]"
	return `{"id":"m","role":"assistant","model":"x","stop_reason":"tool_use","type":"message","content":` + c + `,"usage":{"input_tokens":10,"output_tokens":5}}`
}

func joinCSV(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += ","
		}
		out += v
	}
	return out
}

// ---------------------------------------------------------------------------

func TestGM_PlainTextOnly(t *testing.T) {
	llm := &fakeLLM{
		scripts: []string{msgWith(textBlk("foggy harbor at dusk"))},
	}
	g, err := New(Config{
		LLM: llm, Model: "test-model", SystemPrompt: "be a GM",
		Handler: func(_ context.Context, _ string, _ json.RawMessage) (any, bool) {
			t.Fatal("handler should not be called")
			return nil, false
		},
	})
	require.NoError(t, err)

	trace, hist, err := g.Respond(context.Background(), nil, "look around")
	require.NoError(t, err)
	assert.Equal(t, "foggy harbor at dusk", trace.Narrative)
	assert.Empty(t, trace.ToolCalls)
	assert.Equal(t, 1, trace.Iterations)
	assert.False(t, trace.Truncated)
	assert.GreaterOrEqual(t, len(hist), 2)
}

func TestGM_ToolUseLoop(t *testing.T) {
	// 第 1 轮：assistant 文本 + 一次 roll_skill；第 2 轮：返回最终文本结束。
	script1 := msgWith(
		textBlk("Roll Spot Hidden:"),
		toolUseBlk("tu_1", "roll_skill", map[string]any{
			"skill_name": "Spot Hidden", "skill_value": 60, "difficulty": "regular",
		}),
	)
	script2 := msgWith(textBlk("You see a glint in the corner."))
	llm := &fakeLLM{scripts: []string{script1, script2}}

	handlerCalled := 0
	handler := func(_ context.Context, name string, raw json.RawMessage) (any, bool) {
		handlerCalled++
		assert.Equal(t, "roll_skill", name)
		assert.Contains(t, string(raw), "Spot Hidden")
		return map[string]any{"degree": "regular_success", "success": true}, false
	}

	g, err := New(Config{LLM: llm, Handler: handler})
	require.NoError(t, err)

	trace, _, err := g.Respond(context.Background(), nil, "search the room")
	require.NoError(t, err)
	assert.Equal(t, 1, handlerCalled)
	require.Len(t, trace.ToolCalls, 1)
	assert.Equal(t, "roll_skill", trace.ToolCalls[0].Name)
	assert.Equal(t, 1, trace.ToolCalls[0].Iter)
	assert.Equal(t, 2, trace.Iterations)
	assert.Contains(t, trace.Narrative, "Roll Spot Hidden")
	assert.Contains(t, trace.Narrative, "glint in the corner")
	assert.Equal(t, int64(20), trace.InputTokens)
	assert.Equal(t, int64(10), trace.OutputTokens)
}

func TestGM_TruncationOnLoop(t *testing.T) {
	// 永远只返回 tool_use → 触发 maxIter 截断。
	loop := msgWith(toolUseBlk("tu", "roll_damage", map[string]any{"expression": "1d6"}))
	scripts := make([]string, 20)
	for i := range scripts {
		scripts[i] = loop
	}
	llm := &fakeLLM{scripts: scripts}
	handler := func(_ context.Context, _ string, _ json.RawMessage) (any, bool) {
		return map[string]any{"total": 3}, false
	}
	g, err := New(Config{LLM: llm, Handler: handler, MaxIterations: 3})
	require.NoError(t, err)

	trace, _, err := g.Respond(context.Background(), nil, "x")
	require.NoError(t, err)
	assert.True(t, trace.Truncated)
	assert.Equal(t, 3, trace.Iterations)
	assert.Len(t, trace.ToolCalls, 3)
}

func TestGM_HandlerErrorPropagates(t *testing.T) {
	// handler 标 isError → ToolCall.IsError = true，循环继续，不算致命。
	script1 := msgWith(toolUseBlk("tu", "get_npc", map[string]any{"npc_id": "missing"}))
	script2 := msgWith(textBlk("never mind"))
	llm := &fakeLLM{scripts: []string{script1, script2}}
	handler := func(_ context.Context, _ string, _ json.RawMessage) (any, bool) {
		return map[string]any{"error": "not found"}, true
	}
	g, err := New(Config{LLM: llm, Handler: handler})
	require.NoError(t, err)

	trace, _, err := g.Respond(context.Background(), nil, "x")
	require.NoError(t, err)
	require.Len(t, trace.ToolCalls, 1)
	assert.True(t, trace.ToolCalls[0].IsError)
	assert.Equal(t, "never mind", trace.Narrative)
}

func TestGM_LLMError(t *testing.T) {
	llm := &fakeLLM{failOn: 1, failErr: errors.New("network down")}
	g, err := New(Config{LLM: llm, Handler: func(_ context.Context, _ string, _ json.RawMessage) (any, bool) {
		return nil, false
	}})
	require.NoError(t, err)
	_, _, err = g.Respond(context.Background(), nil, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network down")
}

func TestGM_NewValidates(t *testing.T) {
	_, err := New(Config{Handler: func(_ context.Context, _ string, _ json.RawMessage) (any, bool) { return nil, false }})
	assert.ErrorContains(t, err, "LLM is required")

	_, err = New(Config{LLM: &fakeLLM{}})
	assert.ErrorContains(t, err, "Handler is required")
}

func TestRenderGMSystem(t *testing.T) {
	s, err := RenderGMSystem(PromptContext{
		ScenarioContext:   "Fog Harbor",
		InvestigatorBrief: "Lyra, journalist",
		PlayerGuidance:    "查看公告栏",
	})
	require.NoError(t, err)
	assert.Contains(t, s, "Fog Harbor")
	assert.Contains(t, s, "Lyra, journalist")
	assert.Contains(t, s, "玩家引导素材")
	assert.Contains(t, s, "查看公告栏")
	assert.NotContains(t, s, "Recent events", "empty section should be skipped")
}

func TestNewAnthropic_APIKey(t *testing.T) {
	a := NewAnthropic(ClientConfig{APIKey: "sk-test"})
	assert.NotNil(t, a)
}

func TestNewAnthropic_OpenRouter(t *testing.T) {
	a := NewAnthropic(ClientConfig{
		AuthToken: "or-test",
		BaseURL:   OpenRouterBaseURL,
	})
	assert.NotNil(t, a)
}

func TestNewAnthropic_Empty(t *testing.T) {
	// 不给凭据也能构造（实际调用才会失败），用于 smoke 路径
	a := NewAnthropic(ClientConfig{})
	assert.NotNil(t, a)
}

func TestOpenRouterModel(t *testing.T) {
	assert.Equal(t, "anthropic/claude-sonnet-4-5-20250929",
		string(OpenRouterModel(ModelGM)))
	// 已带前缀不重复加
	prefixed := OpenRouterModel("anthropic/foo")
	assert.Equal(t, "anthropic/foo", string(prefixed))
	// 空模型透传
	assert.Equal(t, "", string(OpenRouterModel("")))
}
