package tools

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDispatcher_AllHandlersRejectBadJSON 用同一份坏 JSON 走每个需要 input 的 handler，
// 确保 decode 路径都被覆盖。
func TestDispatcher_AllHandlersRejectBadJSON(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	bad := json.RawMessage("{")
	for _, name := range []string{
		"roll_skill", "roll_damage", "sanity_check", "opposed_roll",
		"get_npc", "get_location", "list_npcs_at_location",
		"update_investigator_vitals", "update_npc_relation", "kill_npc",
		"mark_location_visited", "move_item", "destroy_item",
		"mark_clue_found", "transition_location", "add_event",
	} {
		_, isErr := d.Dispatch(ctx, name, bad)
		assert.True(t, isErr, "handler %s should reject malformed JSON", name)
	}
}

// TestDispatcher_StoreErrorsBubble 让 store 层返回 ErrNotFound，验证每个写 handler 都包成 isError=true。
func TestDispatcher_StoreErrorsBubble(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	type kase struct {
		name  string
		input string
	}
	cases := []kase{
		{"update_investigator_vitals", `{"investigator_id":"missing","hp":1,"mp":1,"san":1}`},
		{"update_npc_relation", `{"npc_id":"missing","delta":1}`},
		{"kill_npc", `{"npc_id":"missing"}`},
		{"mark_location_visited", `{"location_id":"missing"}`},
		{"move_item", `{"item_id":"missing","owner_type":"none"}`},
		{"destroy_item", `{"item_id":"missing"}`},
		{"mark_clue_found", `{"clue_id":"missing","location_id":"x"}`},
	}
	for _, c := range cases {
		_, isErr := d.Dispatch(ctx, c.name, json.RawMessage(c.input))
		assert.True(t, isErr, "%s on missing entity should be isError", c.name)
	}
}

// TestDispatcher_GetLocation_NotFound 与 list_npcs_at_location 在不存在地点上行为
// （前者 NotFound，后者返回空数组）。
func TestDispatcher_GetLocation_NotFound(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	_, isErr := d.Dispatch(ctx, "get_location", json.RawMessage(`{"location_id":"missing"}`))
	assert.True(t, isErr)

	out, isErr := d.Dispatch(ctx, "list_npcs_at_location", json.RawMessage(`{"location_id":"missing"}`))
	assert.False(t, isErr)
	b, _ := json.Marshal(out)
	assert.Equal(t, "[]", string(b))
}

// TestDispatcher_SanityCheck_NoInvestigator 无 active 调查员 → isError
func TestDispatcher_SanityCheck_NoInvestigator(t *testing.T) {
	s, d, _, ctx := newTestEnv(t)
	_ = s.Repo().DeactivateInvestigator(ctx, "inv-1")
	_, isErr := d.Dispatch(ctx, "sanity_check", json.RawMessage(`{"loss_pass":"0","loss_fail":"1d4"}`))
	assert.True(t, isErr)
}

// TestDispatcher_SanityCheck_BadExpression
//
// 通过把 SAN 推高到 99 强制走 lossPass 分支；坏表达式应冒泡为 isError。
func TestDispatcher_SanityCheck_BadExpression(t *testing.T) {
	s, d, _, ctx := newTestEnv(t)
	require.NoError(t, s.Repo().UpdateInvestigatorVitals(ctx, "inv-1", 12, 10, 99))
	_, isErr := d.Dispatch(ctx, "sanity_check", json.RawMessage(`{"loss_pass":"abc","loss_fail":"1d4"}`))
	assert.True(t, isErr)
}

// TestDispatcher_GetInvestigator_NoActive
func TestDispatcher_GetInvestigator_NoActive(t *testing.T) {
	s, d, _, ctx := newTestEnv(t)
	_ = s.Repo().DeactivateInvestigator(ctx, "inv-1")
	_, isErr := d.Dispatch(ctx, "get_investigator", json.RawMessage(`{}`))
	assert.True(t, isErr)
}
