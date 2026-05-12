package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/agent"
	"github.com/zhuzhenwu/whisperer/internal/memory"
)

// stubLLM 返回固定的 NPC 台词。
type stubLLM struct {
	text string
	err  error
}

func (s *stubLLM) NewMessage(_ context.Context, _ anthropic.MessageNewParams) (*anthropic.Message, error) {
	if s.err != nil {
		return nil, s.err
	}
	body, _ := json.Marshal(map[string]any{
		"id": "m", "role": "assistant", "model": "x", "type": "message", "stop_reason": "end_turn",
		"content": []map[string]any{{"type": "text", "text": s.text}},
		"usage":   map[string]int{"input_tokens": 1, "output_tokens": 1},
	})
	var msg anthropic.Message
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func TestNPCSpeak_NoAgent_Errors(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	_, isErr := d.Dispatch(ctx, "npc_speak", json.RawMessage(`{"npc_id":"npc-1","intent":"x"}`))
	assert.True(t, isErr)
}

func TestNPCSpeak_FullPath(t *testing.T) {
	s, d, saveID, ctx := newTestEnv(t)

	mem, err := memory.New("", memory.NewFakeEmbedder(0))
	require.NoError(t, err)
	require.NoError(t, mem.UpsertEvent(ctx, "evt-prior", "Vance was seen arguing with the harbor master", map[string]string{"npc_id": "npc-1"}))
	d.WithMemory(mem)

	llm := &stubLLM{text: "「关于那件事，我没有什么可说的。」"}
	d.WithNPCAgent(agent.NewNPC(llm, ""))

	out, isErr := d.Dispatch(ctx, "npc_speak", json.RawMessage(`{"npc_id":"npc-1","intent":"deflect","player_line":"你看到什么了？"}`))
	require.False(t, isErr)
	b, _ := json.Marshal(out)
	assert.Contains(t, string(b), "没有什么可说的")
	assert.Contains(t, string(b), `"npc_id":"npc-1"`)

	// 事件应已落库
	events, err := s.Repo().ListEvents(ctx, saveID, 0, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Contains(t, events[0].Description, "Vance:")
	assert.Contains(t, events[0].Description, "没有什么可说的")
}

func TestNPCSpeak_DeadNPCRejected(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	require.NoError(t, d.repo.KillNPC(ctx, "npc-1"))
	d.WithNPCAgent(agent.NewNPC(&stubLLM{text: "x"}, ""))
	_, isErr := d.Dispatch(ctx, "npc_speak", json.RawMessage(`{"npc_id":"npc-1","intent":"x"}`))
	assert.True(t, isErr)
}

func TestNPCSpeak_MissingNPC(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	d.WithNPCAgent(agent.NewNPC(&stubLLM{text: "x"}, ""))
	_, isErr := d.Dispatch(ctx, "npc_speak", json.RawMessage(`{"npc_id":"nope","intent":"x"}`))
	assert.True(t, isErr)
}

func TestNPCSpeak_RequiredFields(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	d.WithNPCAgent(agent.NewNPC(&stubLLM{text: "x"}, ""))
	_, isErr := d.Dispatch(ctx, "npc_speak", json.RawMessage(`{"npc_id":"","intent":""}`))
	assert.True(t, isErr)
}

func TestNPCSpeak_NoMemoryStillWorks(t *testing.T) {
	s, d, saveID, ctx := newTestEnv(t)
	d.WithNPCAgent(agent.NewNPC(&stubLLM{text: "「沉默。」"}, ""))
	// 故意不注入 memory
	_, isErr := d.Dispatch(ctx, "npc_speak", json.RawMessage(`{"npc_id":"npc-1","intent":"x"}`))
	require.False(t, isErr)
	events, _ := s.Repo().ListEvents(ctx, saveID, 0, 0)
	assert.Len(t, events, 1)
}

func TestNPCSpeak_BadJSON(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	d.WithNPCAgent(agent.NewNPC(&stubLLM{text: "x"}, ""))
	_, isErr := d.Dispatch(ctx, "npc_speak", json.RawMessage(`{`))
	assert.True(t, isErr)
}

func TestNPCSpeak_AgentError(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	d.WithNPCAgent(agent.NewNPC(&stubLLM{err: assertErr("upstream down")}, ""))
	_, isErr := d.Dispatch(ctx, "npc_speak", json.RawMessage(`{"npc_id":"npc-1","intent":"x"}`))
	assert.True(t, isErr)
}

type sentinelErr string

func (e sentinelErr) Error() string { return string(e) }

func assertErr(s string) error { return sentinelErr(s) }
