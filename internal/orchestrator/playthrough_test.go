package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

func TestFogHarbor_Playthrough_SolvedPath(t *testing.T) {
	llm := &fakeLLM{scripts: fogHarborSolvedScripts()}
	o, st, saveID, ctx := newOrchestrator(t, llm)
	o.cfg.MaxSLARetries = 0
	effective, variantID, err := scenario.SelectVariantByID(o.cfg.Scenario, "vance_executes")
	require.NoError(t, err)
	o.cfg.Scenario = effective
	o.cfg.VariantID = variantID

	var res TurnResult
	run := func(input string) TurnResult {
		t.Helper()
		out, err := o.RunTurn(ctx, input)
		require.NoError(t, err)
		require.True(t, out.SLAReport.Passed, "input %q", input)
		require.NotEqual(t, "action_guard", firstCheckCode(out.Decision), "input %q narrative %q", input, out.Narrative)
		return out
	}

	res = run("查看公告栏和潮汐表")
	assert.True(t, foundClue(t, st, ctx, saveID, "blood_letter"))
	assert.True(t, foundClue(t, st, ctx, saveID, "tide_chart"))

	res = run("去酒馆")
	assertLocation(t, st, ctx, saveID, "pub")

	res = run("等到晚上并观察账册")
	assertStage(t, st, ctx, saveID, "investigation")
	assert.True(t, foundClue(t, st, ctx, saveID, "ledger"))

	res = run("返回码头")
	assertLocation(t, st, ctx, saveID, "harbor")

	res = run("去灯塔")
	assertFired(t, res, "lighthouse_storm")

	for i := 0; i < 10; i++ {
		res = run("继续观察灯塔与潮声")
	}
	assertStage(t, st, ctx, saveID, "confrontation")
	assertThreatState(t, o, ctx, saveID, "reef_window", "open")

	res = run("进入礁洞")
	assertLocation(t, st, ctx, saveID, "reef_cave")
	assert.True(t, foundClue(t, st, ctx, saveID, "reef_carvings"))
	assert.True(t, foundClue(t, st, ctx, saveID, "sacrifice_chamber"))

	res = run("回到灯塔")
	assertLocation(t, st, ctx, saveID, "lighthouse")

	res = run("返回码头")
	assertLocation(t, st, ctx, saveID, "harbor")

	res = run("去酒馆对峙范斯")
	require.NotNil(t, res.Ending)
	require.NotNil(t, res.Report)
	assert.Equal(t, "solved", res.Ending.ID)
	assert.Equal(t, "success", res.Ending.Kind)
	assertFired(t, res, "culprit_confronted")
	assert.True(t, reportHasClue(res.Report.FoundKeyClues, "blood_letter"))
	assert.True(t, reportHasClue(res.Report.FoundKeyClues, "ledger"))
	assert.True(t, reportHasClue(res.Report.FoundKeyClues, "sacrifice_chamber"))
	assert.Equal(t, "范斯医生", res.Report.CulpritName)
	assert.NotEmpty(t, res.Report.Threats)

	sv, err := st.Repo().GetSave(ctx, saveID)
	require.NoError(t, err)
	assert.Less(t, sv.TurnCount, 22)
}

func fogHarborSolvedScripts() []string {
	scripts := []string{}
	addToolTurn := func(narrative, toolName string, input map[string]any, ending string) {
		scripts = append(scripts,
			msgWith("tool_use", textBlk(narrative), toolUseBlk("tu", toolName, input)),
			msgWith("end_turn", textBlk(ending)),
		)
	}
	addMultiToolTurn := func(narrative string, tools ...string) {
		content := []string{textBlk(narrative)}
		content = append(content, tools...)
		scripts = append(scripts,
			msgWith("tool_use", content...),
			msgWith("end_turn", textBlk("记录已经落下。")),
		)
	}
	addTextTurn := func(text string) {
		scripts = append(scripts, msgWith("end_turn", textBlk(text)))
	}

	addMultiToolTurn("公告栏边缘夹着血迹与潮汐记录。",
		toolUseBlk("tu1", "mark_clue_found", map[string]any{"clue_id": "blood_letter", "location_id": "harbor"}),
		toolUseBlk("tu2", "mark_clue_found", map[string]any{"clue_id": "tide_chart", "location_id": "harbor"}),
	)
	addToolTurn("你离开码头前往酒馆。", "transition_location", map[string]any{"location_id": "pub"}, "钨灯酒馆的门在雾里亮着。")
	addToolTurn("你等到酒馆入夜。", "advance_time", map[string]any{"stages": 2}, "夜色压低，柜台后的账册变得醒目。")
	addToolTurn("你返回码头。", "transition_location", map[string]any{"location_id": "harbor"}, "潮声重新贴近木桩。")
	addToolTurn("你沿北岬路前往灯塔。", "transition_location", map[string]any{"location_id": "lighthouse"}, "灯塔下方传来低鸣。")
	for i := 0; i < 10; i++ {
		addTextTurn("你继续观察潮水与灯塔基座。")
	}
	addMultiToolTurn("你进入礁洞，记录岩壁与圆坑。",
		toolUseBlk("tu3", "transition_location", map[string]any{"location_id": "reef_cave"}),
		toolUseBlk("tu4", "mark_clue_found", map[string]any{"clue_id": "reef_carvings", "location_id": "reef_cave"}),
		toolUseBlk("tu5", "mark_clue_found", map[string]any{"clue_id": "sacrifice_chamber", "location_id": "reef_cave"}),
	)
	addToolTurn("你从礁洞退回灯塔。", "transition_location", map[string]any{"location_id": "lighthouse"}, "风雨仍在塔外旋转。")
	addToolTurn("你回到码头。", "transition_location", map[string]any{"location_id": "harbor"}, "码头的雾已经变得稀薄。")
	addToolTurn("你带着证据去酒馆对峙范斯。", "transition_location", map[string]any{"location_id": "pub"}, "范斯终于沉默。")
	return scripts
}

func foundClue(t *testing.T, st *store.Store, ctx context.Context, saveID, clueID string) bool {
	t.Helper()
	found, err := st.Repo().ListFoundClues(ctx, saveID)
	require.NoError(t, err)
	for _, clue := range found {
		if clue.ID == clueID {
			return true
		}
	}
	return false
}

func assertLocation(t *testing.T, st *store.Store, ctx context.Context, saveID, locationID string) {
	t.Helper()
	sv, err := st.Repo().GetSave(ctx, saveID)
	require.NoError(t, err)
	assert.Equal(t, locationID, sv.CurrentLocationID)
}

func assertStage(t *testing.T, st *store.Store, ctx context.Context, saveID, stage string) {
	t.Helper()
	sv, err := st.Repo().GetSave(ctx, saveID)
	require.NoError(t, err)
	assert.Equal(t, stage, sv.Stage)
}

func assertFired(t *testing.T, res TurnResult, id string) {
	t.Helper()
	for _, fired := range res.Fired {
		if fired.ID == id {
			return
		}
	}
	t.Fatalf("expected trigger %s to fire, got %#v", id, res.Fired)
}

func assertThreatState(t *testing.T, o *Orchestrator, ctx context.Context, saveID, threatID, stateID string) {
	t.Helper()
	threats, err := o.Engine().Threats(ctx, saveID)
	require.NoError(t, err)
	for _, threat := range threats {
		if threat.ID == threatID {
			assert.Equal(t, stateID, threat.StateID)
			return
		}
	}
	t.Fatalf("expected threat %s", threatID)
}

func reportHasClue(clues []scenario.ReportClue, id string) bool {
	for _, clue := range clues {
		if clue.ID == id {
			return true
		}
	}
	return false
}

func firstCheckCode(decision TurnDecision) string {
	if len(decision.Checks) == 0 {
		return ""
	}
	return decision.Checks[0].Code
}
