package scenario

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/store"
)

func newEngineEnv(t *testing.T) (*Engine, *store.Store, string, context.Context) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	r := s.Repo()
	saveID := uuid.NewString()
	require.NoError(t, r.CreateSave(ctx, store.Save{ID: saveID, Name: "t", ScenarioID: "fog_harbor"}))

	scn, err := LoadBundled("fog_harbor")
	require.NoError(t, err)

	return New(scn, r, nil), s, saveID, ctx
}

func TestEngine_Apply_PopulatesStore(t *testing.T) {
	e, s, saveID, ctx := newEngineEnv(t)
	require.NoError(t, e.Apply(ctx, saveID))

	r := s.Repo()
	sv, err := r.GetSave(ctx, saveID)
	require.NoError(t, err)
	assert.Equal(t, "harbor", sv.CurrentLocationID)
	assert.Equal(t, store.TimeMorning, sv.TimeOfDay)
	assert.Equal(t, DefaultStage, sv.Stage)

	loc, err := r.GetLocation(ctx, "harbor")
	require.NoError(t, err)
	assert.True(t, loc.Visited)

	npc, err := r.GetNPC(ctx, "vance")
	require.NoError(t, err)
	assert.Equal(t, "范斯医生", npc.Name)
	assert.True(t, npc.Alive)

	it, err := r.GetItem(ctx, "lantern")
	require.NoError(t, err)
	assert.Equal(t, store.OwnerLocation, it.OwnerType)
}

func TestEngine_Apply_Idempotent(t *testing.T) {
	e, _, saveID, ctx := newEngineEnv(t)
	require.NoError(t, e.Apply(ctx, saveID))
	require.NoError(t, e.Apply(ctx, saveID))
}

func TestEngine_Evaluate_FiresAndDeduplicates(t *testing.T) {
	e, s, saveID, ctx := newEngineEnv(t)
	require.NoError(t, e.Apply(ctx, saveID))

	r := s.Repo()

	// 满足 lighthouse_storm 触发条件
	require.NoError(t, r.MarkClueFound(ctx, "blood_letter", "harbor", 1))
	require.NoError(t, r.UpsertLocation(ctx, store.Location{
		ID: "lighthouse", SaveID: saveID, Name: "灯塔", Description: "x", Visited: true,
	}))

	fired, err := e.Evaluate(ctx, saveID)
	require.NoError(t, err)
	// lighthouse_storm 命中后，cloth_at_low_tide 的 (harbor visited + lighthouse_storm fired)
	// 条件也立刻满足并在同一 Evaluate 内级联触发。
	firedIDs := map[string]bool{}
	for _, f := range fired {
		firedIDs[f.ID] = true
	}
	assert.True(t, firedIDs["lighthouse_storm"], "lighthouse_storm should fire")

	// 第二次不应再触发同一批 trigger
	fired2, err := e.Evaluate(ctx, saveID)
	require.NoError(t, err)
	assert.Empty(t, fired2)
}

func TestEngine_Evaluate_TimeAndRelationActions(t *testing.T) {
	e, s, saveID, ctx := newEngineEnv(t)
	require.NoError(t, e.Apply(ctx, saveID))
	r := s.Repo()

	// 切到 night 让 pub_after_dark 命中
	require.NoError(t, r.SetTimeOfDay(ctx, saveID, store.TimeNight))
	require.NoError(t, r.UpdateSaveProgress(ctx, saveID, "pub", 1))
	require.NoError(t, r.MarkLocationVisited(ctx, "pub"))

	fired, err := e.Evaluate(ctx, saveID)
	require.NoError(t, err)
	ids := []string{}
	for _, f := range fired {
		ids = append(ids, f.ID)
	}
	assert.Contains(t, ids, "pub_after_dark")

	// 关系应被 +5
	npc, _ := r.GetNPC(ctx, "marisa")
	assert.Equal(t, 10, npc.RelationToPlayer) // 初始 5 + 5

	// 账册应已被 mark_clue_found
	clues, _ := r.ListFoundClues(ctx, saveID)
	foundLedger := false
	for _, c := range clues {
		if c.ID == "ledger" {
			foundLedger = true
		}
	}
	assert.True(t, foundLedger)

	sv, _ := r.GetSave(ctx, saveID)
	assert.Equal(t, "investigation", sv.Stage)
}

func TestEngine_SetStageAction(t *testing.T) {
	yamlStr := `
id: tiny
title: t
objectives:
  - stage: opening
    title: 开局
  - stage: confrontation
    title: 对峙
locations: [{id: a, name: A, description: x}]
clues: []
npcs: []
start: {location: a}
key_clues: []
triggers:
  - id: stage
    when: {turn_ge: 1}
    then:
      - set_stage: confrontation
`
	scn, err := Parse([]byte(yamlStr))
	require.NoError(t, err)

	ctx := context.Background()
	s, err := store.Open(ctx, ":memory:")
	require.NoError(t, err)
	defer s.Close()
	r := s.Repo()
	saveID := uuid.NewString()
	require.NoError(t, r.CreateSave(ctx, store.Save{ID: saveID, Name: "x", ScenarioID: "tiny"}))

	e := New(scn, r, nil)
	require.NoError(t, e.Apply(ctx, saveID))
	require.NoError(t, r.UpdateSaveProgress(ctx, saveID, "a", 1))

	fired, err := e.Evaluate(ctx, saveID)
	require.NoError(t, err)
	require.Len(t, fired, 1)

	sv, _ := r.GetSave(ctx, saveID)
	assert.Equal(t, "confrontation", sv.Stage)
}

func TestEngine_CheckEndings(t *testing.T) {
	e, s, saveID, ctx := newEngineEnv(t)
	require.NoError(t, e.Apply(ctx, saveID))
	r := s.Repo()

	// 触发 victim_dies：把 helena 杀掉
	require.NoError(t, r.KillNPC(ctx, "helena"))
	end, err := e.CheckEndings(ctx, saveID)
	require.NoError(t, err)
	require.NotNil(t, end)
	assert.Equal(t, "victim_dies", end.ID)
	assert.Equal(t, "failure", end.Kind)
}

func TestEngine_CheckEndings_NoneWhenIncomplete(t *testing.T) {
	e, _, saveID, ctx := newEngineEnv(t)
	require.NoError(t, e.Apply(ctx, saveID))
	end, err := e.CheckEndings(ctx, saveID)
	require.NoError(t, err)
	assert.Nil(t, end)
}

func TestEngine_AdvanceTimeAction(t *testing.T) {
	// 用一个最小自定义剧本，含 advance_time action
	yamlStr := `
id: tiny
title: t
locations: [{id: a, name: A, description: x}]
clues: []
npcs: []
start: {location: a}
key_clues: []
triggers:
  - id: tick
    when: {turn_ge: 1}
    then:
      - advance_time: 2
`
	scn, err := Parse([]byte(yamlStr))
	require.NoError(t, err)

	ctx := context.Background()
	s, err := store.Open(ctx, ":memory:")
	require.NoError(t, err)
	defer s.Close()
	r := s.Repo()
	saveID := uuid.NewString()
	require.NoError(t, r.CreateSave(ctx, store.Save{ID: saveID, Name: "x", ScenarioID: "tiny"}))

	e := New(scn, r, nil)
	require.NoError(t, e.Apply(ctx, saveID))
	require.NoError(t, r.UpdateSaveProgress(ctx, saveID, "a", 1))

	fired, err := e.Evaluate(ctx, saveID)
	require.NoError(t, err)
	require.Len(t, fired, 1)

	sv, _ := r.GetSave(ctx, saveID)
	assert.Equal(t, store.TimeNight, sv.TimeOfDay) // morning + 2 = night
}

func TestEngine_Threats(t *testing.T) {
	e, s, saveID, ctx := newEngineEnv(t)
	require.NoError(t, e.Apply(ctx, saveID))
	r := s.Repo()

	statuses, err := e.Threats(ctx, saveID)
	require.NoError(t, err)
	require.NotEmpty(t, statuses)
	assert.Equal(t, "安娜危险", statuses[0].Name)
	assert.Equal(t, "尚未卷入", statuses[0].StateLabel)

	require.NoError(t, r.MarkClueFound(ctx, "anna_warning", "pub", 1))
	statuses, err = e.Threats(ctx, saveID)
	require.NoError(t, err)
	found := false
	for _, status := range statuses {
		if status.ID == "anna_danger" {
			found = true
			assert.Equal(t, "被盯上", status.StateLabel)
			assert.Equal(t, 1, status.Severity)
		}
	}
	assert.True(t, found)
}
