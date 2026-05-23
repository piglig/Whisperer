package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePlayerAction_MoveToScenarioLocation(t *testing.T) {
	o, _, saveID, ctx := newOrchestrator(t, &fakeLLM{})

	action := parsePlayerAction(ctx, o.cfg.Store.Repo(), saveID, o.cfg.Scenario, "前往钨灯酒馆")

	assert.Equal(t, IntentMove, action.Kind)
	assert.Equal(t, "前往钨灯酒馆", action.Text)
	require.True(t, hasActionTarget(action, "location", "pub"))
}

func TestParsePlayerAction_TalkBracketTarget(t *testing.T) {
	o, _, saveID, ctx := newOrchestrator(t, &fakeLLM{})

	action := parsePlayerAction(ctx, o.cfg.Store.Repo(), saveID, o.cfg.Scenario, "[talk:vance] 你看到露西了吗？")

	assert.Equal(t, IntentTalk, action.Kind)
	assert.Equal(t, "你看到露西了吗？", action.Text)
	require.True(t, hasActionTarget(action, "npc", "vance"))
}

func TestParsePlayerAction_UseItem(t *testing.T) {
	o, _, saveID, ctx := newOrchestrator(t, &fakeLLM{})

	action := parsePlayerAction(ctx, o.cfg.Store.Repo(), saveID, o.cfg.Scenario, "点亮黄铜油灯检查礁洞")

	assert.Equal(t, IntentUseItem, action.Kind)
	require.True(t, hasActionTarget(action, "item", "lantern"))
	require.True(t, hasActionTarget(action, "location", "reef_cave"))
}

func TestParsePlayerAction_EncodedAction(t *testing.T) {
	o, _, saveID, ctx := newOrchestrator(t, &fakeLLM{})
	encoded := EncodePlayerAction(PlayerAction{
		Kind: IntentTalk,
		Raw:  "询问范斯",
		Text: "询问范斯，露西失踪前是否来找过他。",
		Targets: []ActionTarget{{
			Kind: "npc",
			ID:   "vance",
			Name: "伊莱亚斯·范斯",
		}},
	})

	action := parsePlayerAction(ctx, o.cfg.Store.Repo(), saveID, o.cfg.Scenario, encoded)

	assert.Equal(t, IntentTalk, action.Kind)
	assert.Equal(t, "询问范斯，露西失踪前是否来找过他。", action.Text)
	require.True(t, hasActionTarget(action, "npc", "vance"))
}

func hasActionTarget(action PlayerAction, kind, id string) bool {
	for _, target := range action.Targets {
		if target.Kind == kind && target.ID == id {
			return true
		}
	}
	return false
}
