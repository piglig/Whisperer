package scenario

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/memory"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

// 各 condition 分支的针对性单测：覆盖 evalCondition 与 validateCondition 的边角。

func mustCondition(t *testing.T, c Condition, v stateView, want bool) {
	t.Helper()
	got, err := evalCondition(c, v)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestEvalCondition_All_Any_Not(t *testing.T) {
	v := stateView{
		VisitedLocs:  map[string]bool{"a": true},
		FoundClues:   map[string]bool{"c1": true},
		DeadNPCs:     map[string]bool{"n1": true},
		NPCRelations: map[string]int{"n1": 10, "n2": -30},
		Fired:        map[string]bool{"t1": true},
		Save:         store.Save{TimeOfDay: store.TimeNight, TurnCount: 5},
	}

	mustCondition(t, Condition{LocationVisited: "a"}, v, true)
	mustCondition(t, Condition{LocationVisited: "b"}, v, false)
	mustCondition(t, Condition{ClueFound: "c1"}, v, true)
	mustCondition(t, Condition{NPCDead: "n1"}, v, true)
	mustCondition(t, Condition{NPCDead: "n2"}, v, false)
	mustCondition(t, Condition{NPCRelationLT: &RelChk{NPC: "n2", Value: 0}}, v, true)
	mustCondition(t, Condition{NPCRelationGT: &RelChk{NPC: "n1", Value: 5}}, v, true)
	mustCondition(t, Condition{TimeOfDay: "night"}, v, true)
	mustCondition(t, Condition{TimeOfDay: "morning"}, v, false)
	mustCondition(t, Condition{TurnGE: 5}, v, true)
	mustCondition(t, Condition{TurnGE: 6}, v, false)
	mustCondition(t, Condition{TriggerFired: "t1"}, v, true)

	// location_in: 命中任一即真
	vLoc := v
	vLoc.Save.CurrentLocationID = "pub"
	mustCondition(t, Condition{LocationIn: []string{"pub", "reef_cave"}}, vLoc, true)
	mustCondition(t, Condition{LocationIn: []string{"reef_cave", "pub"}}, vLoc, true)
	mustCondition(t, Condition{LocationIn: []string{"church"}}, vLoc, false)

	// All / Any / Not 组合
	mustCondition(t, Condition{All: []Condition{
		{LocationVisited: "a"}, {ClueFound: "c1"},
	}}, v, true)
	mustCondition(t, Condition{All: []Condition{
		{LocationVisited: "a"}, {ClueFound: "missing"},
	}}, v, false)
	mustCondition(t, Condition{Any: []Condition{
		{LocationVisited: "missing"}, {ClueFound: "c1"},
	}}, v, true)
	mustCondition(t, Condition{Any: []Condition{
		{LocationVisited: "missing"}, {ClueFound: "missing"},
	}}, v, false)
	mustCondition(t, Condition{Not: &Condition{LocationVisited: "missing"}}, v, true)
	mustCondition(t, Condition{Not: &Condition{LocationVisited: "a"}}, v, false)
}

func TestEvalCondition_UnknownReturnsError(t *testing.T) {
	_, err := evalCondition(Condition{}, stateView{})
	assert.Error(t, err)
}

func TestApply_WithMemory(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, ":memory:")
	require.NoError(t, err)
	defer s.Close()
	r := s.Repo()
	saveID := uuid.NewString()
	require.NoError(t, r.CreateSave(ctx, store.Save{ID: saveID, Name: "x", ScenarioID: "fog_harbor"}))

	mem, err := memory.New("", memory.NewFakeEmbedder(0))
	require.NoError(t, err)

	scn, err := LoadBundled("fog_harbor")
	require.NoError(t, err)
	e := New(scn, r, mem)
	require.NoError(t, e.Apply(ctx, saveID))

	// memory 中应有所有 NPC 档案；至少包含 vance / father_calvin（v0.3.0 新增）
	hits, err := mem.QueryNPCs(ctx, "范斯", len(scn.NPCs))
	require.NoError(t, err)
	require.Len(t, hits, len(scn.NPCs))
	ids := map[string]bool{}
	for _, h := range hits {
		ids[h.ID] = true
	}
	assert.True(t, ids["vance"], "vance profile should be present")
	assert.True(t, ids["father_calvin"], "father_calvin profile should be present")

	// 线索描述也应进入 clues collection（v0.3.0 含 12 条 = 11 主线 + 1 red herring）
	chits, err := mem.QueryClues(ctx, "letter", len(scn.Clues))
	require.NoError(t, err)
	require.Len(t, chits, len(scn.Clues))
}

func TestEngine_CheckEndings_TriggerFiredCondition(t *testing.T) {
	e, s, saveID, ctx := newEngineEnv(t)
	require.NoError(t, e.Apply(ctx, saveID))
	r := s.Repo()

	// 满足 solved 结局（v0.3.0）：sacrifice_chamber + reef_carvings + ledger 找到，
	// 且 vance_confronted 触发器已 fire。
	require.NoError(t, r.MarkClueFound(ctx, "ledger", "pub", 1))
	require.NoError(t, r.MarkClueFound(ctx, "reef_carvings", "reef_cave", 1))
	require.NoError(t, r.MarkClueFound(ctx, "sacrifice_chamber", "reef_cave", 1))
	// culprit_confronted 触发条件：找到 reef_carvings + 当前回到 pub
	require.NoError(t, r.UpdateSaveProgress(ctx, saveID, "pub", 1))
	require.NoError(t, r.MarkLocationVisited(ctx, "pub"))
	_, err := e.Evaluate(ctx, saveID) // 触发 vance_confronted
	require.NoError(t, err)

	end, err := e.CheckEndings(ctx, saveID)
	require.NoError(t, err)
	require.NotNil(t, end)
	assert.Equal(t, "solved", end.ID)
}

// 自定义剧本验证 advance_time / kill_npc / update_npc_relation 三种 action 的执行路径
func TestEngine_AllActionTypes(t *testing.T) {
	yamlStr := `
id: t
title: t
locations: [{id: a, name: A, description: x}]
clues: [{id: c1, description: z}]
npcs: [{id: n1, name: N, personality: p}]
start: {location: a}
key_clues: []
triggers:
  - id: t1
    when: {turn_ge: 1}
    then:
      - kill_npc: n1
  - id: t2
    when: {turn_ge: 1}
    then:
      - update_npc_relation: {npc_id: n1, delta: 50}
  - id: t3
    when: {turn_ge: 1}
    then:
      - mark_clue_found: {clue_id: c1, location_id: a}
`
	scn, err := Parse([]byte(yamlStr))
	require.NoError(t, err)

	ctx := context.Background()
	s, _ := store.Open(ctx, ":memory:")
	defer s.Close()
	r := s.Repo()
	saveID := uuid.NewString()
	require.NoError(t, r.CreateSave(ctx, store.Save{ID: saveID, Name: "x", ScenarioID: "t"}))

	e := New(scn, r, nil)
	require.NoError(t, e.Apply(ctx, saveID))
	require.NoError(t, r.UpdateSaveProgress(ctx, saveID, "a", 1))

	fired, err := e.Evaluate(ctx, saveID)
	require.NoError(t, err)
	assert.Len(t, fired, 3)

	// kill_npc: t1 必须先于 t2 update_npc_relation；engine 按声明顺序，t1 → kill → relation 更新仍可写入死掉的 NPC（store 不阻止）
	npc, _ := r.GetNPC(ctx, "n1")
	assert.False(t, npc.Alive)
	assert.Equal(t, 50, npc.RelationToPlayer)

	clues, _ := r.ListFoundClues(ctx, saveID)
	assert.Len(t, clues, 1)
}
