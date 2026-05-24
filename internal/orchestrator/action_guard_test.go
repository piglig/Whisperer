package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuardPlayerAction_RejectsDisconnectedMove(t *testing.T) {
	o, _, saveID, ctx := newOrchestrator(t, &fakeLLM{})
	action := PlayerAction{
		Kind: IntentMove,
		Raw:  "去礁洞",
		Text: "去礁洞",
		Targets: []ActionTarget{{
			Kind: "location",
			ID:   "reef_cave",
			Name: "灯塔下礁洞",
		}},
	}

	result := guardPlayerAction(ctx, o.cfg.Store.Repo(), saveID, o.cfg.Scenario, action)

	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "不能直接前往")
	require.NotEmpty(t, result.Suggestions)
}

func TestGuardPlayerAction_RejectsRemoteNPC(t *testing.T) {
	o, _, saveID, ctx := newOrchestrator(t, &fakeLLM{})
	action := PlayerAction{
		Kind: IntentTalk,
		Raw:  "问范斯",
		Text: "问范斯",
		Targets: []ActionTarget{{
			Kind: "npc",
			ID:   "vance",
			Name: "范斯医生",
		}},
	}

	result := guardPlayerAction(ctx, o.cfg.Store.Repo(), saveID, o.cfg.Scenario, action)

	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "不在当前地点")
	assert.Contains(t, result.Suggestions[0], "钨灯酒馆")
}

func TestGuardPlayerAction_RejectsUnavailableItem(t *testing.T) {
	o, _, saveID, ctx := newOrchestrator(t, &fakeLLM{})
	action := PlayerAction{
		Kind: IntentUseItem,
		Raw:  "使用教区登记簿手抄本",
		Text: "使用教区登记簿手抄本",
		Targets: []ActionTarget{{
			Kind: "item",
			ID:   "parish_copy",
			Name: "教区登记簿手抄本",
		}},
	}

	result := guardPlayerAction(ctx, o.cfg.Store.Repo(), saveID, o.cfg.Scenario, action)

	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "不在你能直接使用的位置")
}

func TestGuardPlayerAction_AllowsCurrentItemActionSource(t *testing.T) {
	o, _, saveID, ctx := newOrchestrator(t, &fakeLLM{})
	action := PlayerAction{
		Kind:   IntentUseItem,
		Raw:    "拿起油灯",
		Text:   "拿起黄铜油灯并检查灯芯。",
		Source: ActionSource{Kind: "item_action", ID: "take_lantern"},
		Targets: []ActionTarget{{
			Kind: "item",
			ID:   "lantern",
			Name: "黄铜油灯",
		}},
	}

	result := guardPlayerAction(ctx, o.cfg.Store.Repo(), saveID, o.cfg.Scenario, action)

	assert.True(t, result.Allowed)
}

func TestGuardPlayerAction_RejectsStaleItemActionSource(t *testing.T) {
	o, _, saveID, ctx := newOrchestrator(t, &fakeLLM{})
	action := PlayerAction{
		Kind:   IntentUseItem,
		Raw:    "点亮油灯检查礁洞",
		Text:   "点亮黄铜油灯，先照亮礁洞入口。",
		Source: ActionSource{Kind: "item_action", ID: "light_for_cave"},
		Targets: []ActionTarget{{
			Kind: "item",
			ID:   "lantern",
			Name: "黄铜油灯",
		}},
	}

	result := guardPlayerAction(ctx, o.cfg.Store.Repo(), saveID, o.cfg.Scenario, action)

	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "物品行动当前不可用")
}

func TestGuardPlayerAction_RejectsStaleDialogueSource(t *testing.T) {
	o, _, saveID, ctx := newOrchestrator(t, &fakeLLM{})
	action := PlayerAction{
		Kind:   IntentTalk,
		Raw:    "问范斯",
		Text:   "问范斯。",
		Source: ActionSource{Kind: "dialogue_option", ID: "missing_option"},
		Targets: []ActionTarget{{
			Kind: "npc",
			ID:   "vance",
			Name: "范斯医生",
		}},
	}

	result := guardPlayerAction(ctx, o.cfg.Store.Repo(), saveID, o.cfg.Scenario, action)

	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "对话选项当前不可用")
}

func TestRunTurn_GuardRejectionSkipsLLMAndTurnAdvance(t *testing.T) {
	llm := &fakeLLM{scripts: []string{msgWith("end_turn", textBlk("不应该调用"))}}
	o, st, saveID, ctx := newOrchestrator(t, llm)
	action := EncodePlayerAction(PlayerAction{
		Kind: IntentMove,
		Raw:  "去礁洞",
		Text: "去礁洞",
		Targets: []ActionTarget{{
			Kind: "location",
			ID:   "reef_cave",
			Name: "灯塔下礁洞",
		}},
	})

	res, err := o.RunTurn(ctx, action)
	require.NoError(t, err)

	assert.Zero(t, llm.calls)
	assert.Contains(t, res.Narrative, "行动未执行")
	require.Len(t, res.Decision.Checks, 1)
	assert.Equal(t, "location_not_connected", res.Decision.Checks[0].Code)
	assert.False(t, res.Decision.Checks[0].Passed)
	assert.Equal(t, ActionBlocked, res.Decision.ActionDecision.Status)
	assert.Equal(t, "location_not_connected", res.Decision.ActionDecision.ReasonCode)
	assert.Contains(t, res.Decision.ActionDecision.PlayerFacingReason, "不能直接前往")
	require.NotEmpty(t, res.Decision.ActionDecision.SuggestedActions)
	sv, err := st.Repo().GetSave(ctx, saveID)
	require.NoError(t, err)
	assert.Zero(t, sv.TurnCount)
}
