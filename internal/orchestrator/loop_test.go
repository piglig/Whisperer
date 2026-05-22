package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/agent"
	"github.com/zhuzhenwu/whisperer/internal/memory"
	"github.com/zhuzhenwu/whisperer/internal/orchestrator/sla"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

// fakeLLM 把固定脚本回放为 *agent.Message。每个 script 是已 marshal 的 message JSON。
type fakeLLM struct {
	scripts []string
	calls   int
	err     error
}

func (f *fakeLLM) NewMessage(_ context.Context, _ agent.MessageRequest) (*agent.Message, error) {
	if f.err != nil {
		return nil, f.err
	}
	idx := f.calls
	f.calls++
	if idx >= len(f.scripts) {
		// 默认结束：空文本，end_turn
		return parseMsg(`{"id":"end","role":"assistant","model":"x","stop_reason":"end_turn","type":"message","content":[{"type":"text","text":""}],"usage":{"input_tokens":1,"output_tokens":1}}`), nil
	}
	return parseMsg(f.scripts[idx]), nil
}

func parseMsg(s string) *agent.Message {
	var m agent.Message
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		panic(err)
	}
	return &m
}

func textBlk(text string) string {
	b, _ := json.Marshal(map[string]any{"type": "text", "text": text})
	return string(b)
}

func toolUseBlk(id, name string, input map[string]any) string {
	b, _ := json.Marshal(map[string]any{"type": "tool_use", "id": id, "name": name, "input": input})
	return string(b)
}

func msgWith(stop string, content ...string) string {
	c := "[" + joinCSV(content) + "]"
	return `{"id":"m","role":"assistant","model":"x","stop_reason":"` + stop + `","type":"message","content":` + c + `,"usage":{"input_tokens":5,"output_tokens":5}}`
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

func newOrchestrator(t *testing.T, llm *fakeLLM) (*Orchestrator, *store.Store, string, context.Context) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	saveID := uuid.NewString()
	r := st.Repo()
	require.NoError(t, r.CreateSave(ctx, store.Save{ID: saveID, Name: "t", ScenarioID: "fog_harbor"}))
	require.NoError(t, r.UpsertInvestigator(ctx, store.Investigator{
		ID: "inv-1", SaveID: saveID, Name: "Lyra", Occupation: "Journalist",
		AttrsJSON: "{}", SkillsJSON: "{}", InventoryJSON: "[]",
		HP: 12, MP: 12, SAN: 60, Active: true,
	}))

	scn, err := scenario.LoadBundled("fog_harbor")
	require.NoError(t, err)

	mem, err := memory.New("", memory.NewFakeEmbedder(0))
	require.NoError(t, err)

	o, err := New(Config{
		Store:    st,
		Memory:   mem,
		Scenario: scn,
		LLMGM:    llm,
		LLMNPC:   llm,
		SaveID:   saveID,
		RNG:      rand.New(rand.NewPCG(1, 2)),
	})
	require.NoError(t, err)

	require.NoError(t, o.Engine().Apply(ctx, saveID))
	return o, st, saveID, ctx
}

func TestNew_Validates(t *testing.T) {
	_, err := New(Config{})
	assert.ErrorContains(t, err, "Store is required")

	_, err = New(Config{Store: &store.Store{}})
	assert.ErrorContains(t, err, "Scenario is required")
}

func TestRunTurn_HappyPath(t *testing.T) {
	// 单回合：GM 调一次 roll_skill 然后给出最终叙事。
	llm := &fakeLLM{scripts: []string{
		msgWith("tool_use",
			textBlk("Roll Spot Hidden:"),
			toolUseBlk("tu1", "roll_skill", map[string]any{
				"skill_name": "Spot Hidden", "skill_value": 50, "difficulty": "regular",
			}),
		),
		msgWith("end_turn", textBlk("你看到桌上有一封信。")),
	}}
	o, st, saveID, ctx := newOrchestrator(t, llm)

	res, err := o.RunTurn(ctx, "我搜查桌面")
	require.NoError(t, err)
	assert.Contains(t, res.Narrative, "你看到桌上有一封信")
	assert.Empty(t, res.SLAReport.Violations)
	assert.NotNil(t, res.Trace.ToolCalls)

	// turn count 应推进
	sv, _ := st.Repo().GetSave(ctx, saveID)
	assert.Equal(t, 1, sv.TurnCount)

	// events 应至少含 narrative 与 roll
	events, _ := st.Repo().ListEvents(ctx, saveID, 0, 0)
	assert.NotEmpty(t, events)
}

// TestRunTurn_TransitionLocationPersists 防回归 W7-bug：
// dispatcher 在 LLM 循环里调 transition_location 后，回合末推进 turn count
// 不应把新 location 覆盖回老的。
func TestRunTurn_TransitionLocationPersists(t *testing.T) {
	llm := &fakeLLM{scripts: []string{
		msgWith("tool_use",
			textBlk("移动到酒馆。"),
			toolUseBlk("tu1", "transition_location", map[string]any{
				"location_id": "pub",
			}),
		),
		msgWith("end_turn", textBlk("你抵达酒馆。")),
	}}
	o, st, saveID, ctx := newOrchestrator(t, llm)

	_, err := o.RunTurn(ctx, "去酒馆")
	require.NoError(t, err)

	sv, err := st.Repo().GetSave(ctx, saveID)
	require.NoError(t, err)
	assert.Equal(t, "pub", sv.CurrentLocationID,
		"transition_location 写入的新 location 必须保留，不能被回合末推进 turn count 时覆盖")
	assert.Equal(t, 1, sv.TurnCount)
}

func TestRunTurn_TriggersFire(t *testing.T) {
	// 让 GM 找到 blood_letter 并访问 lighthouse → 应触发 lighthouse_storm。
	llm := &fakeLLM{scripts: []string{
		msgWith("tool_use",
			textBlk("调查码头。"),
			toolUseBlk("tu1", "mark_clue_found", map[string]any{
				"clue_id": "blood_letter", "location_id": "harbor",
			}),
			toolUseBlk("tu2", "transition_location", map[string]any{
				"location_id": "lighthouse",
			}),
		),
		msgWith("end_turn", textBlk("你抵达灯塔。")),
	}}
	o, _, _, ctx := newOrchestrator(t, llm)

	res, err := o.RunTurn(ctx, "去灯塔")
	require.NoError(t, err)
	ids := []string{}
	for _, f := range res.Fired {
		ids = append(ids, f.ID)
	}
	assert.Contains(t, ids, "lighthouse_storm")
}

func TestRunTurn_EndingForcedOnZeroHP(t *testing.T) {
	llm := &fakeLLM{scripts: []string{
		msgWith("tool_use",
			textBlk("致命一击。"),
			toolUseBlk("tu1", "update_investigator_vitals", map[string]any{
				"investigator_id": "inv-1", "hp": 0, "mp": 0, "san": 0,
			}),
		),
		msgWith("end_turn", textBlk("黑暗笼罩。")),
	}}
	o, _, _, ctx := newOrchestrator(t, llm)

	res, err := o.RunTurn(ctx, "硬冲")
	require.NoError(t, err)
	require.NotNil(t, res.Ending)
	assert.Equal(t, "failure", res.Ending.Kind)
	assert.True(t, res.SLAReport.EndingForced)
	require.NotNil(t, res.Report)
	assert.Equal(t, "investigator_lost", res.Report.Ending.ID)
}

func TestRunTurn_SLARetry(t *testing.T) {
	// 第一次：narrative 含 "成功" 但无 roll_* → SLA #1 违规
	// 第二次：GM 不再使用 "成功" 关键词 → 通过
	llm := &fakeLLM{scripts: []string{
		msgWith("end_turn", textBlk("你成功推开了门。")),
		msgWith("end_turn", textBlk("你推门进入。")),
	}}
	o, _, _, ctx := newOrchestrator(t, llm)
	o.cfg.MaxSLARetries = 2

	res, err := o.RunTurn(ctx, "推门")
	require.NoError(t, err)
	assert.True(t, res.SLAReport.Passed)
	assert.Contains(t, res.Narrative, "推门进入")
}

func TestRunTurn_SLARetryRollsBackFailedAttemptWrites(t *testing.T) {
	llm := &fakeLLM{scripts: []string{
		msgWith("tool_use",
			textBlk("你成功抵达酒馆。"),
			toolUseBlk("tu1", "transition_location", map[string]any{
				"location_id": "pub",
			}),
		),
		msgWith("end_turn", textBlk("你成功抵达酒馆。")),
		msgWith("end_turn", textBlk("你留在原地，重新观察门缝。")),
	}}
	o, st, saveID, ctx := newOrchestrator(t, llm)
	o.cfg.MaxSLARetries = 1

	res, err := o.RunTurn(ctx, "去酒馆")
	require.NoError(t, err)
	require.True(t, res.SLAReport.Passed)
	assert.Contains(t, res.Narrative, "重新观察")

	sv, err := st.Repo().GetSave(ctx, saveID)
	require.NoError(t, err)
	assert.Equal(t, "harbor", sv.CurrentLocationID, "failed SLA attempt transition must be rolled back")
}

func TestRunTurn_SLAFallback(t *testing.T) {
	// 一直违规 → 用尽重试 → 接受最后一次输出，Passed=false
	llm := &fakeLLM{scripts: []string{
		msgWith("end_turn", textBlk("你成功推开了门。")),
		msgWith("end_turn", textBlk("你成功扭开锁芯。")),
		msgWith("end_turn", textBlk("你成功通过。")),
	}}
	o, _, _, ctx := newOrchestrator(t, llm)
	o.cfg.MaxSLARetries = 2

	res, err := o.RunTurn(ctx, "推门")
	require.NoError(t, err)
	assert.False(t, res.SLAReport.Passed)
}

func TestRunTurn_LLMError_RollsBack(t *testing.T) {
	llm := &fakeLLM{err: errors.New("network down")}
	o, st, saveID, ctx := newOrchestrator(t, llm)
	preTurn := mustSave(t, st, ctx, saveID).TurnCount

	_, err := o.RunTurn(ctx, "x")
	require.Error(t, err)

	postTurn := mustSave(t, st, ctx, saveID).TurnCount
	assert.Equal(t, preTurn, postTurn, "turn count must not advance on error")
}

func TestHistoryAndReset(t *testing.T) {
	llm := &fakeLLM{scripts: []string{msgWith("end_turn", textBlk("ok"))}}
	o, _, _, ctx := newOrchestrator(t, llm)
	_, err := o.RunTurn(ctx, "say hi")
	require.NoError(t, err)
	require.NotEmpty(t, o.History())
	o.ResetHistory()
	assert.Empty(t, o.History())
}

func TestSaveIDExposed(t *testing.T) {
	llm := &fakeLLM{}
	o, _, saveID, _ := newOrchestrator(t, llm)
	assert.Equal(t, saveID, o.SaveID())
}

// spyJudge 不返回违规，只计数。用于覆盖 buildJudgeContext + CheckWithJudge 路径。
type spyJudge struct {
	npcCalls, knowCalls int
}

func (s *spyJudge) JudgeNPCConsistency(_ context.Context, _ sla.JudgeNPCInput) (*sla.Violation, error) {
	s.npcCalls++
	return nil, nil
}

func (s *spyJudge) JudgeKnowledgeProjection(_ context.Context, _ sla.JudgeKnowledgeInput) (*sla.Violation, error) {
	s.knowCalls++
	return nil, nil
}

func (s *spyJudge) JudgeFailureContradiction(_ context.Context, _ sla.JudgeFailureInput) (*sla.Violation, error) {
	return nil, nil
}

func TestRunTurn_WithJudge_NPCCallTriggers(t *testing.T) {
	llm := &fakeLLM{scripts: []string{
		msgWith("tool_use",
			textBlk("范斯说话:"),
			toolUseBlk("tu1", "npc_speak", map[string]any{
				"npc_id": "vance", "intent": "deflect", "player_line": "你看到什么？",
			}),
		),
		msgWith("end_turn", textBlk("沉默蔓延。")),
	}}
	o, _, _, ctx := newOrchestrator(t, llm)

	// 给 dispatcher 注入 NPCAgent，让 npc_speak 真的能跑（用同一个 fakeLLM）
	// 这里通过另一个脚本：reuse llm.scripts 末尾自动返回空文本
	llm.scripts = append(llm.scripts, msgWith("end_turn", textBlk("「无可奉告。」")))

	spy := &spyJudge{}
	o.cfg.Judge = spy
	o.cfg.LLMNPC = llm

	res, err := o.RunTurn(ctx, "问范斯")
	require.NoError(t, err)
	npcSeen := false
	for _, tc := range res.Trace.ToolCalls {
		if tc.Name == "npc_speak" && !tc.IsError {
			npcSeen = true
		}
	}
	if npcSeen {
		assert.GreaterOrEqual(t, spy.npcCalls, 1)
	}
}

func TestRunTurn_WithJudge_KnowledgeNarrative(t *testing.T) {
	llm := &fakeLLM{scripts: []string{
		msgWith("end_turn", textBlk("你想起一段古老的传说。")),
	}}
	o, _, _, ctx := newOrchestrator(t, llm)
	spy := &spyJudge{}
	o.cfg.Judge = spy

	_, err := o.RunTurn(ctx, "回忆")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, spy.knowCalls, 1)
}

func TestStringIndex_Helper(t *testing.T) {
	assert.Equal(t, 0, stringIndex("abc", "abc"))
	assert.Equal(t, 1, stringIndex("xabc", "abc"))
	assert.Equal(t, -1, stringIndex("abc", "z"))
}

func mustSave(t *testing.T, st *store.Store, ctx context.Context, id string) store.Save {
	t.Helper()
	sv, err := st.Repo().GetSave(ctx, id)
	require.NoError(t, err)
	return sv
}
