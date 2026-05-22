package sla

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/agent"
)

// fakeJudge 让我们对 CheckWithJudge 的派发逻辑做断言，不依赖真实 LLM。
type fakeJudge struct {
	npcReturn  *Violation
	npcErr     error
	knowReturn *Violation
	knowErr    error
	failReturn *Violation
	failErr    error
	npcCalls   int
	knowCalls  int
	failCalls  int
	lastNPCIn  JudgeNPCInput
	lastKnowIn JudgeKnowledgeInput
	lastFailIn JudgeFailureInput
}

func (f *fakeJudge) JudgeNPCConsistency(_ context.Context, in JudgeNPCInput) (*Violation, error) {
	f.npcCalls++
	f.lastNPCIn = in
	return f.npcReturn, f.npcErr
}
func (f *fakeJudge) JudgeKnowledgeProjection(_ context.Context, in JudgeKnowledgeInput) (*Violation, error) {
	f.knowCalls++
	f.lastKnowIn = in
	return f.knowReturn, f.knowErr
}
func (f *fakeJudge) JudgeFailureContradiction(_ context.Context, in JudgeFailureInput) (*Violation, error) {
	f.failCalls++
	f.lastFailIn = in
	return f.failReturn, f.failErr
}

func TestCheckWithJudge_NilJudge_FallsBackToStructural(t *testing.T) {
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	r := v.CheckWithJudge(context.Background(), agent.TurnTrace{Narrative: "雾很浓。"}, nil, JudgeContext{})
	assert.True(t, r.Passed)
}

func TestCheckWithJudge_NPCInconsistencyDetected(t *testing.T) {
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	out := map[string]any{"dialogue": "开心地说话", "npc_id": "vance"}
	b, _ := json.Marshal(out)
	trace := agent.TurnTrace{
		Narrative: "x",
		ToolCalls: []agent.ToolCall{{Name: "npc_speak", Output: b}},
	}
	j := &fakeJudge{
		npcReturn: &Violation{Code: CodeNPCInconsistent, Message: "口吻不符"},
	}
	jctx := JudgeContext{NPCPersonas: map[string]string{"vance": "stern"}}
	r := v.CheckWithJudge(context.Background(), trace, j, jctx)
	assert.False(t, r.Passed)
	assert.Equal(t, 1, j.npcCalls)
	assert.Equal(t, "vance", j.lastNPCIn.NPCID)
}

func TestCheckWithJudge_NPCSpeakWithError_Skipped(t *testing.T) {
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	trace := agent.TurnTrace{
		Narrative: "x",
		ToolCalls: []agent.ToolCall{{Name: "npc_speak", IsError: true, Output: json.RawMessage(`{}`)}},
	}
	j := &fakeJudge{}
	r := v.CheckWithJudge(context.Background(), trace, j, JudgeContext{})
	assert.True(t, r.Passed)
	assert.Equal(t, 0, j.npcCalls)
}

func TestCheckWithJudge_KnowledgeOverreachDetected(t *testing.T) {
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	trace := agent.TurnTrace{Narrative: "你想起这是阿撒托斯的低语。"}
	j := &fakeJudge{
		knowReturn: &Violation{Code: CodeKnowledgeOverreach, Message: "no Cthulhu Mythos"},
	}
	r := v.CheckWithJudge(context.Background(), trace, j,
		JudgeContext{InvestigatorSkillsJSON: `{"Library Use":40}`, InvestigatorOccupation: "记者"})
	assert.False(t, r.Passed)
	assert.Equal(t, 1, j.knowCalls)
	assert.Contains(t, j.lastKnowIn.Narrative, "你想起")
}

func TestCheckWithJudge_NoTrigger_NoCalls(t *testing.T) {
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	trace := agent.TurnTrace{Narrative: "雾很浓。"}
	j := &fakeJudge{}
	r := v.CheckWithJudge(context.Background(), trace, j, JudgeContext{})
	assert.True(t, r.Passed)
	assert.Equal(t, 0, j.npcCalls)
	assert.Equal(t, 0, j.knowCalls)
}

func TestCheckWithJudge_FailureContradictionFalsePositiveRemoved(t *testing.T) {
	// L1 字符级误判：narrative 含 "成功" 但被复合句否定，Judge 判为通过 → 应清除 violation。
	fail := false
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	out := map[string]any{"success": fail}
	b, _ := json.Marshal(out)
	trace := agent.TurnTrace{
		Narrative: "你成功靠近了她，但她依旧拒绝了你的请求。", // 复合句：靠近成功但说服失败
		ToolCalls: []agent.ToolCall{{Name: "roll_skill", Output: b}},
	}
	// L1 会误标为违规
	r0 := v.Check(trace)
	hasL1 := false
	for _, vio := range r0.Violations {
		if vio.Code == CodeFailureContradict {
			hasL1 = true
		}
	}
	require.True(t, hasL1, "前置：L1 应当误标")

	// Judge 判定通过（返回 nil） → 应清除
	j := &fakeJudge{failReturn: nil, failErr: nil}
	r := v.CheckWithJudge(context.Background(), trace, j, JudgeContext{})
	for _, vio := range r.Violations {
		assert.NotEqual(t, CodeFailureContradict, vio.Code, "Judge 判通过应清除 L1 假阳")
	}
	assert.Equal(t, 1, j.failCalls)
	assert.Equal(t, "roll_skill", j.lastFailIn.ToolName)
}

func TestCheckWithJudge_FailureContradictionConfirmed(t *testing.T) {
	// L1 误标为违规，但 Judge 也判违规 → 用 Judge 的 message
	fail := false
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	out := map[string]any{"success": fail}
	b, _ := json.Marshal(out)
	trace := agent.TurnTrace{
		Narrative: "你成功扭开了锁。",
		ToolCalls: []agent.ToolCall{{Name: "roll_skill", Output: b}},
	}
	j := &fakeJudge{failReturn: &Violation{
		Code:    CodeFailureContradict,
		Message: "narrative 明确表达成功",
	}}
	r := v.CheckWithJudge(context.Background(), trace, j, JudgeContext{})
	codes := []Code{}
	msgs := []string{}
	for _, vio := range r.Violations {
		codes = append(codes, vio.Code)
		msgs = append(msgs, vio.Message)
	}
	assert.Contains(t, codes, CodeFailureContradict)
	// 应使用 Judge 的 message
	hasJudgeMsg := false
	for _, m := range msgs {
		if m == "narrative 明确表达成功" {
			hasJudgeMsg = true
		}
	}
	assert.True(t, hasJudgeMsg, "应保留 Judge 给的 message 而非 L1 模板")
}

func TestCheckWithJudge_FailureContradictionJudgeError_KeepsL1(t *testing.T) {
	fail := false
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	out := map[string]any{"success": fail}
	b, _ := json.Marshal(out)
	trace := agent.TurnTrace{
		Narrative: "你成功扭开了锁。",
		ToolCalls: []agent.ToolCall{{Name: "roll_skill", Output: b}},
	}
	j := &fakeJudge{failErr: errors.New("upstream")}
	r := v.CheckWithJudge(context.Background(), trace, j, JudgeContext{})
	codes := []Code{}
	for _, vio := range r.Violations {
		codes = append(codes, vio.Code)
	}
	assert.Contains(t, codes, CodeFailureContradict, "Judge 失败 → 保守保留 L1 判定")
}

func TestCheckWithJudge_FailureContradictionNoL1NoJudgeCall(t *testing.T) {
	// L1 没标违规（narrative 中规中矩），不应调 Judge
	fail := false
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	out := map[string]any{"success": fail}
	b, _ := json.Marshal(out)
	trace := agent.TurnTrace{
		Narrative: "你尝试推开门，但她抢先一步关上了。",
		ToolCalls: []agent.ToolCall{{Name: "roll_skill", Output: b}},
	}
	j := &fakeJudge{}
	r := v.CheckWithJudge(context.Background(), trace, j, JudgeContext{})
	assert.True(t, r.Passed)
	assert.Equal(t, 0, j.failCalls, "L1 没命中时不应调 Judge")
}

func TestCheckWithJudge_JudgeError_DoesNotFail(t *testing.T) {
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	out := map[string]any{"dialogue": "x", "npc_id": "vance"}
	b, _ := json.Marshal(out)
	trace := agent.TurnTrace{
		Narrative: "x",
		ToolCalls: []agent.ToolCall{{Name: "npc_speak", Output: b}},
	}
	j := &fakeJudge{npcErr: errors.New("upstream")}
	r := v.CheckWithJudge(context.Background(), trace, j, JudgeContext{})
	// 错误被吞，Report 仍 pass（结构化没违规）
	assert.True(t, r.Passed)
}

// HaikuJudge: 用 fakeLLM 验证解析路径。

type fakeJudgeLLM struct {
	text string
	err  error
}

func (f *fakeJudgeLLM) NewMessage(_ context.Context, _ agent.MessageRequest) (*agent.Message, error) {
	if f.err != nil {
		return nil, f.err
	}
	body, _ := json.Marshal(map[string]any{
		"id": "m", "role": "assistant", "model": "x", "type": "message", "stop_reason": "end_turn",
		"content": []map[string]any{{"type": "text", "text": f.text}},
		"usage":   map[string]int{"input_tokens": 1, "output_tokens": 1},
	})
	var msg agent.Message
	require.NoError(nil, json.Unmarshal(body, &msg))
	return &msg, nil
}

func TestHaikuJudge_NPCViolation(t *testing.T) {
	llm := &fakeJudgeLLM{text: `{"violation":true,"reason":"口吻完全变了"}`}
	j := NewHaikuJudge(llm, "")
	v, err := j.JudgeNPCConsistency(context.Background(), JudgeNPCInput{NPCID: "vance"})
	require.NoError(t, err)
	require.NotNil(t, v)
	assert.Equal(t, CodeNPCInconsistent, v.Code)
}

func TestHaikuJudge_NPCPasses(t *testing.T) {
	llm := &fakeJudgeLLM{text: `{"violation":false}`}
	j := NewHaikuJudge(llm, "")
	v, err := j.JudgeNPCConsistency(context.Background(), JudgeNPCInput{NPCID: "vance"})
	require.NoError(t, err)
	assert.Nil(t, v)
}

func TestHaikuJudge_KnowledgeViolation(t *testing.T) {
	llm := &fakeJudgeLLM{text: `{"violation":true,"reason":"超出技能"}`}
	j := NewHaikuJudge(llm, "")
	v, err := j.JudgeKnowledgeProjection(context.Background(), JudgeKnowledgeInput{})
	require.NoError(t, err)
	require.NotNil(t, v)
	assert.Equal(t, CodeKnowledgeOverreach, v.Code)
}

func TestHaikuJudge_UnparseableOutput_Passes(t *testing.T) {
	llm := &fakeJudgeLLM{text: "随便写的话"}
	j := NewHaikuJudge(llm, "")
	v, err := j.JudgeNPCConsistency(context.Background(), JudgeNPCInput{})
	require.NoError(t, err)
	assert.Nil(t, v, "unparseable output should be treated as pass")
}

func TestHaikuJudge_LLMError(t *testing.T) {
	llm := &fakeJudgeLLM{err: errors.New("boom")}
	j := NewHaikuJudge(llm, "")
	_, err := j.JudgeNPCConsistency(context.Background(), JudgeNPCInput{})
	assert.Error(t, err)
}
