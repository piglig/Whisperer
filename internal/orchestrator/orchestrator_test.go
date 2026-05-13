package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/orchestrator/sla"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

// 单独覆盖 renderSystemPrompt / snapshotForSLA / buildSLAFeedback / New 的边角。

func TestRenderSystemPrompt_RichSnapshot(t *testing.T) {
	llm := &fakeLLM{}
	o, st, saveID, ctx := newOrchestrator(t, llm)
	r := st.Repo()

	// 推进一些状态：visit harbor、添加事件
	require.NoError(t, r.UpdateSaveProgress(ctx, saveID, "harbor", 2))
	for i := 0; i < 6; i++ {
		_, _ = r.AppendEvent(ctx, store.Event{
			SaveID: saveID, Turn: i + 1, Type: store.EventNarrative,
			Description: "事件 " + string(rune('A'+i)),
		})
	}

	s, err := o.renderSystemPrompt(ctx)
	require.NoError(t, err)
	assert.Contains(t, s, "回合 2")
	assert.Contains(t, s, "Lyra")
	assert.Contains(t, s, "雾港码头")
	assert.Contains(t, s, "事件 F", "应包含最近事件")
	assert.NotContains(t, s, "事件 A", "最早事件应被截断")
}

func TestRenderSystemPrompt_NoActiveInvestigator(t *testing.T) {
	llm := &fakeLLM{}
	o, st, _, ctx := newOrchestrator(t, llm)
	require.NoError(t, st.Repo().DeactivateInvestigator(ctx, "inv-1"))
	s, err := o.renderSystemPrompt(ctx)
	require.NoError(t, err)
	assert.NotContains(t, s, "Lyra")
}

func TestRenderSystemPrompt_NoLocation(t *testing.T) {
	llm := &fakeLLM{}
	o, st, saveID, ctx := newOrchestrator(t, llm)
	// 把 current_location_id 抹掉 by 设置为不存在的位置触发 GetLocation 失败 → 优雅跳过
	require.NoError(t, st.Repo().UpdateSaveProgress(ctx, saveID, "ghost", 0))
	s, err := o.renderSystemPrompt(ctx)
	require.NoError(t, err)
	assert.NotContains(t, s, "Current location:")
}

func TestSnapshotForSLA_DestroyedAndInactive(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	require.NoError(t, err)
	defer st.Close()
	r := st.Repo()

	saveID := uuid.NewString()
	require.NoError(t, r.CreateSave(ctx, store.Save{ID: saveID, Name: "x", ScenarioID: "fh"}))
	require.NoError(t, r.UpsertItem(ctx, store.Item{
		ID: "i1", SaveID: saveID, Name: "黄铜油灯", Description: "x",
		OwnerType: store.OwnerNone, Destroyed: true,
	}))
	snap, err := snapshotForSLA(ctx, r, saveID)
	require.NoError(t, err)
	assert.Equal(t, []string{"黄铜油灯"}, snap.DestroyedItemNames)
	assert.False(t, snap.InvestigatorActive, "no active investigator")
}

func TestBuildSLAFeedback_Empty(t *testing.T) {
	assert.Empty(t, buildSLAFeedback(sla.Report{Passed: true}))
}

func TestBuildSLAFeedback_WithViolations(t *testing.T) {
	r := sla.Report{Violations: []sla.Violation{
		{Code: sla.CodeRollMissing, Message: "missing"},
	}}
	out := buildSLAFeedback(r)
	assert.Contains(t, out, "<sla_violation>")
	assert.Contains(t, out, "missing")
}

func TestNew_RNGAutoFilled(t *testing.T) {
	llm := &fakeLLM{}
	o, _, saveID, _ := newOrchestrator(t, llm)
	o2, err := New(Config{
		Store: o.cfg.Store, Memory: o.cfg.Memory, Scenario: o.cfg.Scenario,
		LLMGM: llm, SaveID: saveID,
		MaxSLARetries: -1, // 应被夹紧到默认 1
	})
	require.NoError(t, err)
	assert.NotNil(t, o2.cfg.RNG)
	assert.Equal(t, 1, o2.cfg.MaxSLARetries)
}

func TestNew_MissingSaveID(t *testing.T) {
	llm := &fakeLLM{}
	o, _, _, _ := newOrchestrator(t, llm)
	_, err := New(Config{
		Store: o.cfg.Store, Memory: o.cfg.Memory, Scenario: o.cfg.Scenario,
		LLMGM: llm, SaveID: "",
	})
	assert.ErrorContains(t, err, "SaveID")
}

func TestNew_MissingLLM(t *testing.T) {
	o, _, saveID, _ := newOrchestrator(t, nil)
	_, err := New(Config{
		Store: o.cfg.Store, Memory: o.cfg.Memory, Scenario: o.cfg.Scenario,
		SaveID: saveID,
	})
	assert.ErrorContains(t, err, "LLMGM")
}

// 适应 buildSLAFeedback 接收 sla.Report 真实类型——但只验证空报表返回空字符串。
// （非空场景已在 RunTurn 测试中走过。）
func TestBindNewInvestigator(t *testing.T) {
	llm := &fakeLLM{}
	o, st, saveID, ctx := newOrchestrator(t, llm)

	// 假装上一局结束：deactivate 当前 inv-1
	require.NoError(t, st.Repo().DeactivateInvestigator(ctx, "inv-1"))

	newInv := store.Investigator{
		ID: "inv-2", Name: "Theo", Occupation: "教士",
		AttrsJSON: `{"INT":80}`, SkillsJSON: `{"Library Use":70}`,
		InventoryJSON: "[]",
		HP:            12, MP: 10, SAN: 65,
	}
	require.NoError(t, o.BindNewInvestigator(ctx, newInv))

	got, err := st.Repo().GetActiveInvestigator(ctx, saveID)
	require.NoError(t, err)
	assert.Equal(t, "inv-2", got.ID)
	assert.True(t, got.Active)

	// 旧 inv 仍在 db，但不再 active
	all, _ := st.Repo().ListInvestigators(ctx, saveID)
	assert.Len(t, all, 2)

	// history 应被清空
	assert.Empty(t, o.History())
}

func TestBindNewInvestigator_RequiresID(t *testing.T) {
	llm := &fakeLLM{}
	o, _, _, ctx := newOrchestrator(t, llm)
	err := o.BindNewInvestigator(ctx, store.Investigator{Name: "no-id"})
	assert.ErrorContains(t, err, "ID")
}

func TestBuildSLAFeedback_NonEmpty(t *testing.T) {
	llm := &fakeLLM{scripts: []string{
		msgWith("end_turn", textBlk("你成功推开了门。")),
		msgWith("end_turn", textBlk("门动了。")),
	}}
	o, _, _, ctx := newOrchestrator(t, llm)
	o.cfg.MaxSLARetries = 2
	res, err := o.RunTurn(ctx, "推门")
	require.NoError(t, err)
	// 第二次输出应不再包含成功语义，且 feedback 至少出现在第二次 LLM 调用的 user side 内容中
	// （fakeLLM 不记录请求，但通过最终 SLAReport.Passed 间接断言）
	assert.True(t, res.SLAReport.Passed)
	assert.False(t, strings.Contains(res.Narrative, "成功"))
}

func TestRenderSystemPrompt_InjectsTruthAndPrior(t *testing.T) {
	llm := &fakeLLM{}
	o, _, _, ctx := newOrchestrator(t, llm)
	// fog_harbor base scenario 在 newOrchestrator 已加载。truth 字段非空。
	out, err := o.renderSystemPrompt(ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "剧本真相")
	assert.Contains(t, out, "深潜者")
	assert.Contains(t, out, "NPC 秘密")
	assert.Contains(t, out, "线索三层网")
	assert.Contains(t, out, "Tier 1")

	// 未配置 Meta 时不出现"玩家先验"段
	assert.NotContains(t, out, "玩家先验")
}

func TestRecordCompletion_PersistsMeta(t *testing.T) {
	llm := &fakeLLM{}
	o, _, _, ctx := newOrchestrator(t, llm)
	tmp := t.TempDir() + "/meta.json"
	o.cfg.Meta = &scenario.MetaState{}
	o.cfg.MetaPath = tmp
	o.cfg.VariantID = "vance_pact"

	require.NoError(t, o.cfg.Store.Repo().MarkClueFound(ctx, "blood_letter", "harbor", 1))

	require.NoError(t, o.RecordCompletion(ctx, "solved"))
	loaded, err := scenario.LoadMeta(tmp)
	require.NoError(t, err)
	assert.Equal(t, 1, loaded.PlayCount)
	assert.Contains(t, loaded.CompletedVariants, "vance_pact")
	assert.Contains(t, loaded.CompletedEndings, "solved")
	assert.Contains(t, loaded.DiscoveredTruths, "blood_letter")
}
