package tools

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/store"
)

func newTestEnv(t *testing.T) (*store.Store, *Dispatcher, string, context.Context) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	r := s.Repo()
	saveID := uuid.NewString()
	require.NoError(t, r.CreateSave(ctx, store.Save{ID: saveID, Name: "t", ScenarioID: "fh"}))
	require.NoError(t, r.UpsertInvestigator(ctx, store.Investigator{
		ID: "inv-1", SaveID: saveID, Name: "Lyra", Occupation: "Journalist",
		AttrsJSON: "{}", SkillsJSON: "{}", InventoryJSON: "[]",
		HP: 12, MP: 10, SAN: 60, Active: true,
	}))
	require.NoError(t, r.UpsertLocation(ctx, store.Location{
		ID: "harbor", SaveID: saveID, Name: "Harbor", Description: "foggy",
	}))
	require.NoError(t, r.UpsertNPC(ctx, store.NPC{
		ID: "npc-1", SaveID: saveID, Name: "Vance", Personality: "stern",
		LocationID: "harbor", Alive: true,
	}))
	require.NoError(t, r.UpsertItem(ctx, store.Item{
		ID: "lantern", SaveID: saveID, Name: "Brass Lantern", Description: "old",
		OwnerType: store.OwnerLocation, OwnerID: "harbor",
	}))
	require.NoError(t, r.UpsertClue(ctx, store.Clue{
		ID: "letter", SaveID: saveID, ScenarioID: "fh", Description: "blood-stained letter",
	}))

	d := New(r, saveID, 3, rand.New(rand.NewPCG(1, 2)))
	return s, d, saveID, ctx
}

func dispatchInto[T any](t *testing.T, d *Dispatcher, ctx context.Context, name string, in any) (T, bool) {
	t.Helper()
	raw, _ := json.Marshal(in)
	out, isErr := d.Dispatch(ctx, name, raw)
	b, _ := json.Marshal(out)
	var v T
	require.NoError(t, json.Unmarshal(b, &v))
	return v, isErr
}

// ---------------------------------------------------------------------------
// Schema sanity
// ---------------------------------------------------------------------------

func TestDispatcher_ToolsContainAll(t *testing.T) {
	_, d, _, _ := newTestEnv(t)
	tools := d.Tools()
	assert.GreaterOrEqual(t, len(tools), 18)
	names := map[string]bool{}
	for _, tool := range tools {
		require.NotEmpty(t, tool.Name)
		names[tool.Name] = true
	}
	for _, expected := range []string{
		"roll_skill", "roll_damage", "sanity_check", "opposed_roll",
		"get_investigator", "get_npc", "get_location", "list_npcs_at_location", "list_found_clues",
		"update_investigator_vitals", "update_npc_relation", "kill_npc",
		"mark_location_visited", "move_item", "destroy_item",
		"mark_clue_found", "transition_location", "add_event",
	} {
		assert.True(t, names[expected], "missing tool: %s", expected)
	}
}

func TestDispatcher_UnknownTool(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	out, isErr := d.Dispatch(ctx, "no_such_tool", json.RawMessage(`{}`))
	assert.True(t, isErr)
	b, _ := json.Marshal(out)
	assert.Contains(t, string(b), "unknown tool")
}

// ---------------------------------------------------------------------------
// rules
// ---------------------------------------------------------------------------

func TestDispatcher_RollSkill(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	out, isErr := d.Dispatch(ctx, "roll_skill", json.RawMessage(`{"skill_name":"x","skill_value":50,"difficulty":"regular"}`))
	require.False(t, isErr)
	b, _ := json.Marshal(out)
	assert.Contains(t, string(b), "degree")
}

func TestDispatcher_RollSkill_BadDifficulty(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	_, isErr := d.Dispatch(ctx, "roll_skill", json.RawMessage(`{"skill_name":"x","skill_value":50,"difficulty":"impossible"}`))
	assert.True(t, isErr)
}

func TestDispatcher_RollDamage(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	out, isErr := d.Dispatch(ctx, "roll_damage", json.RawMessage(`{"expression":"1d6+2"}`))
	require.False(t, isErr)
	b, _ := json.Marshal(out)
	assert.Contains(t, string(b), `"total"`)

	_, isErr = d.Dispatch(ctx, "roll_damage", json.RawMessage(`{"expression":"garbage"}`))
	assert.True(t, isErr)
}

func TestDispatcher_SanityCheck_PersistsSAN(t *testing.T) {
	s, d, saveID, ctx := newTestEnv(t)
	out, isErr := d.Dispatch(ctx, "sanity_check", json.RawMessage(`{"loss_pass":"0","loss_fail":"1d4"}`))
	require.False(t, isErr)
	b, _ := json.Marshal(out)
	assert.Contains(t, string(b), `"new_san"`)

	inv, err := s.Repo().GetActiveInvestigator(ctx, saveID)
	require.NoError(t, err)
	assert.LessOrEqual(t, inv.SAN, 60)
}

func TestDispatcher_SanityCheck_DeactivatesAtZero(t *testing.T) {
	s, d, saveID, ctx := newTestEnv(t)
	// 把 SAN 降到 1 → 一定失败 → loss="1d4" 至少扣 1 → 归零或更低 → deactivate
	require.NoError(t, s.Repo().UpdateInvestigatorVitals(ctx, "inv-1", 12, 10, 1))
	for range 5 {
		_, isErr := d.Dispatch(ctx, "sanity_check", json.RawMessage(`{"loss_pass":"0","loss_fail":"1d10"}`))
		if isErr {
			continue
		}
		inv, err := s.Repo().GetActiveInvestigator(ctx, saveID)
		if err != nil {
			// 已被 deactivate，找不到 active 投查员
			return
		}
		_ = inv
	}
	// 至少应该已 deactivate 一次
	_, err := s.Repo().GetActiveInvestigator(ctx, saveID)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestDispatcher_OpposedRoll(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	out, isErr := d.Dispatch(ctx, "opposed_roll", json.RawMessage(`{"actor_name":"a","actor_skill":60,"target_name":"b","target_skill":40}`))
	require.False(t, isErr)
	b, _ := json.Marshal(out)
	assert.Contains(t, string(b), `"winner"`)
}

// ---------------------------------------------------------------------------
// read
// ---------------------------------------------------------------------------

type invView struct {
	ID, Name string
	HP, SAN  int
}

func TestDispatcher_GetInvestigator(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	v, isErr := dispatchInto[invView](t, d, ctx, "get_investigator", map[string]any{})
	require.False(t, isErr)
	assert.Equal(t, "inv-1", v.ID)
	assert.Equal(t, "Lyra", v.Name)
}

func TestDispatcher_GetNPC(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	type npcView struct {
		ID, Name string
	}
	v, isErr := dispatchInto[npcView](t, d, ctx, "get_npc", map[string]any{"npc_id": "npc-1"})
	require.False(t, isErr)
	assert.Equal(t, "Vance", v.Name)

	_, isErr = d.Dispatch(ctx, "get_npc", json.RawMessage(`{"npc_id":"missing"}`))
	assert.True(t, isErr)
}

func TestDispatcher_GetLocation(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	type locView struct {
		ID, Name string
	}
	v, isErr := dispatchInto[locView](t, d, ctx, "get_location", map[string]any{"location_id": "harbor"})
	require.False(t, isErr)
	assert.Equal(t, "Harbor", v.Name)
}

func TestDispatcher_ListNPCsAtLocation(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	type npcView struct {
		ID string
	}
	out, isErr := d.Dispatch(ctx, "list_npcs_at_location", json.RawMessage(`{"location_id":"harbor"}`))
	require.False(t, isErr)
	b, _ := json.Marshal(out)
	var list []npcView
	require.NoError(t, json.Unmarshal(b, &list))
	require.Len(t, list, 1)
	assert.Equal(t, "npc-1", list[0].ID)
}

func TestDispatcher_ListFoundClues_Empty(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	out, isErr := d.Dispatch(ctx, "list_found_clues", json.RawMessage(`{}`))
	require.False(t, isErr)
	b, _ := json.Marshal(out)
	assert.Equal(t, "[]", string(b))
}

// ---------------------------------------------------------------------------
// write
// ---------------------------------------------------------------------------

func TestDispatcher_UpdateVitals_DeactivatesOnZero(t *testing.T) {
	s, d, saveID, ctx := newTestEnv(t)
	_, isErr := d.Dispatch(ctx, "update_investigator_vitals",
		json.RawMessage(`{"investigator_id":"inv-1","hp":0,"mp":10,"san":50}`))
	require.False(t, isErr)
	_, err := s.Repo().GetActiveInvestigator(ctx, saveID)
	assert.ErrorIs(t, err, store.ErrNotFound, "hp=0 should deactivate")
}

func TestDispatcher_UpdateRelation(t *testing.T) {
	s, d, _, ctx := newTestEnv(t)
	_, isErr := d.Dispatch(ctx, "update_npc_relation", json.RawMessage(`{"npc_id":"npc-1","delta":-15}`))
	require.False(t, isErr)
	npc, err := s.Repo().GetNPC(ctx, "npc-1")
	require.NoError(t, err)
	assert.Equal(t, -15, npc.RelationToPlayer)
}

func TestDispatcher_KillNPC(t *testing.T) {
	s, d, _, ctx := newTestEnv(t)
	_, isErr := d.Dispatch(ctx, "kill_npc", json.RawMessage(`{"npc_id":"npc-1"}`))
	require.False(t, isErr)
	npc, _ := s.Repo().GetNPC(ctx, "npc-1")
	assert.False(t, npc.Alive)
}

func TestDispatcher_MarkLocationVisited(t *testing.T) {
	s, d, _, ctx := newTestEnv(t)
	_, isErr := d.Dispatch(ctx, "mark_location_visited", json.RawMessage(`{"location_id":"harbor"}`))
	require.False(t, isErr)
	loc, _ := s.Repo().GetLocation(ctx, "harbor")
	assert.True(t, loc.Visited)
}

func TestDispatcher_MoveItem(t *testing.T) {
	s, d, _, ctx := newTestEnv(t)
	_, isErr := d.Dispatch(ctx, "move_item", json.RawMessage(`{"item_id":"lantern","owner_type":"investigator","owner_id":"inv-1"}`))
	require.False(t, isErr)
	it, _ := s.Repo().GetItem(ctx, "lantern")
	assert.Equal(t, store.OwnerInvestigator, it.OwnerType)
	assert.Equal(t, "inv-1", it.OwnerID)
}

func TestDispatcher_MoveItem_BadOwnerType(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	_, isErr := d.Dispatch(ctx, "move_item", json.RawMessage(`{"item_id":"lantern","owner_type":"banana"}`))
	assert.True(t, isErr)
}

func TestDispatcher_DestroyItem(t *testing.T) {
	s, d, saveID, ctx := newTestEnv(t)
	_, isErr := d.Dispatch(ctx, "destroy_item", json.RawMessage(`{"item_id":"lantern"}`))
	require.False(t, isErr)
	dest, err := s.Repo().ListDestroyedItems(ctx, saveID)
	require.NoError(t, err)
	require.Len(t, dest, 1)
	assert.Equal(t, "lantern", dest[0].ID)
}

func TestDispatcher_MarkClueFound(t *testing.T) {
	s, d, saveID, ctx := newTestEnv(t)
	_, isErr := d.Dispatch(ctx, "mark_clue_found", json.RawMessage(`{"clue_id":"letter","location_id":"harbor"}`))
	require.False(t, isErr)
	found, err := s.Repo().ListFoundClues(ctx, saveID)
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, 3, found[0].FoundAtTurn) // turn 来自 dispatcher

	// 同时应 emit 一条 "clue_found:<id>" narrative event，让 drift detector 把本
	// 回合识别为主线推进（W7 真实 e2e 暴露的 bug）。
	events, err := s.Repo().ListEvents(ctx, saveID, 0, 0)
	require.NoError(t, err)
	hasProgress := false
	for _, e := range events {
		if e.Type == "narrative" && len(e.Description) >= len("clue_found:") &&
			e.Description[:len("clue_found:")] == "clue_found:" {
			hasProgress = true
			assert.Contains(t, e.RelatedEntitiesJSON, "letter")
		}
	}
	assert.True(t, hasProgress, "mark_clue_found 必须 emit 一条 'clue_found:' 前缀的 narrative")
}

func TestDispatcher_TransitionLocation(t *testing.T) {
	s, d, saveID, ctx := newTestEnv(t)
	_, isErr := d.Dispatch(ctx, "transition_location", json.RawMessage(`{"location_id":"harbor"}`))
	require.False(t, isErr)
	sv, _ := s.Repo().GetSave(ctx, saveID)
	assert.Equal(t, "harbor", sv.CurrentLocationID)
	assert.Equal(t, 3, sv.TurnCount)
	loc, _ := s.Repo().GetLocation(ctx, "harbor")
	assert.True(t, loc.Visited)
}

func TestDispatcher_TransitionLocation_UnknownDestination(t *testing.T) {
	// 目的地不存在 → 不致命，返回 ok+warning
	_, d, _, ctx := newTestEnv(t)
	out, isErr := d.Dispatch(ctx, "transition_location", json.RawMessage(`{"location_id":"unknown"}`))
	require.False(t, isErr)
	b, _ := json.Marshal(out)
	assert.Contains(t, string(b), "warning")
}

func TestDispatcher_AddEvent(t *testing.T) {
	s, d, saveID, ctx := newTestEnv(t)
	_, isErr := d.Dispatch(ctx, "add_event",
		json.RawMessage(`{"type":"narrative","description":"player enters harbor","related_entities":["npc-1"]}`))
	require.False(t, isErr)
	events, err := s.Repo().ListEvents(ctx, saveID, 0, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, store.EventNarrative, events[0].Type)
	assert.Equal(t, 3, events[0].Turn)
}

// ---------------------------------------------------------------------------
// bad input
// ---------------------------------------------------------------------------

func TestDispatcher_BadInputJSON(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	_, isErr := d.Dispatch(ctx, "roll_skill", json.RawMessage(`{`))
	assert.True(t, isErr)
}

func TestDispatcher_SetClock(t *testing.T) {
	_, d, _, _ := newTestEnv(t)
	d.SetClock(func() int64 { return 999 })
	// no observable side effect through repo (which uses its own nowMS), but
	// we exercise the setter for coverage.
}
