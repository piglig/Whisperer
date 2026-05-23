package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildActionBrief(t *testing.T) {
	action := PlayerAction{
		Kind: IntentTalk,
		Raw:  "问范斯",
		Text: "询问范斯露西失踪前是否来过。",
		Source: ActionSource{
			Kind: "dialogue_option",
			ID:   "ask_lucy_visit",
		},
		Targets: []ActionTarget{{
			Kind: "npc",
			ID:   "vance",
			Name: "范斯医生",
		}},
	}
	brief := buildActionBrief(action, ActionGuardResult{Allowed: true, NormalizedAction: action})

	assert.Contains(t, brief, "类型：交谈")
	assert.Contains(t, brief, "npc vance（范斯医生）")
	assert.Contains(t, brief, "来源：dialogue_option ask_lucy_visit")
	assert.Contains(t, brief, "规则门卫：已允许")
	assert.Contains(t, brief, "不要把一次行动扩展")
}
