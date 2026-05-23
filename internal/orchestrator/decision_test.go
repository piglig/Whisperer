package orchestrator

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/agent"
	"github.com/zhuzhenwu/whisperer/internal/orchestrator/sla"
)

func TestBuildTurnDecision_IntentAndStateChanges(t *testing.T) {
	trace := agent.TurnTrace{
		ToolCalls: []agent.ToolCall{
			{
				Name:   "transition_location",
				Input:  rawJSON(t, map[string]any{"location_id": "pub"}),
				Output: rawJSON(t, map[string]any{"ok": true}),
			},
		},
	}
	summary := TurnSummary{
		LocationChange: &ValueChange{From: "雾港码头", To: "钨灯酒馆"},
		NewClues:       []SummaryClue{{ID: "ledger", Description: "账册"}},
		FiredTriggers:  []string{"opening_to_investigation"},
	}

	action := PlayerAction{Kind: IntentMove, Raw: "去酒馆", Text: "去酒馆", Targets: []ActionTarget{{Kind: "location", ID: "pub", Name: "钨灯酒馆"}}}
	decision := buildTurnDecision(action, trace, summary, sla.Report{Passed: true})

	assert.Equal(t, IntentMove, decision.Intent)
	assert.Equal(t, action, decision.Action)
	require.Len(t, decision.Mechanics, 1)
	assert.Equal(t, "transition_location", decision.Mechanics[0].Tool)
	assert.Equal(t, "pub", decision.Mechanics[0].Target)
	assert.True(t, decision.Mechanics[0].Success)
	assert.Contains(t, decision.StateChanges, DecisionChange{Kind: "location", From: "雾港码头", To: "钨灯酒馆"})
	assert.Contains(t, decision.StateChanges, DecisionChange{Kind: "clue", ID: "ledger", Detail: "账册"})
	assert.Contains(t, decision.StateChanges, DecisionChange{Kind: "trigger", ID: "opening_to_investigation"})
	require.Len(t, decision.Checks, 1)
	assert.Equal(t, "rules_applied", decision.Checks[0].Code)
	assert.True(t, decision.Checks[0].Passed)
}

func TestBuildTurnDecision_RecordsRejections(t *testing.T) {
	trace := agent.TurnTrace{
		ToolCalls: []agent.ToolCall{
			{
				Name:    "mark_clue_found",
				Input:   rawJSON(t, map[string]any{"clue_id": "missing"}),
				Output:  rawJSON(t, map[string]any{"error": "clue not found"}),
				IsError: true,
			},
		},
	}
	report := sla.Report{
		Passed: false,
		Violations: []sla.Violation{{
			Code:    sla.CodeRollMissing,
			Message: "missing roll",
		}},
	}

	action := PlayerAction{Kind: IntentUnknown, Raw: "我成功打开锁", Text: "我成功打开锁"}
	decision := buildTurnDecision(action, trace, TurnSummary{}, report)

	require.Len(t, decision.Mechanics, 1)
	assert.False(t, decision.Mechanics[0].Success)
	assert.Equal(t, "clue not found", decision.Mechanics[0].Detail)
	assert.Contains(t, decision.Checks, DecisionCheck{Code: "tool_rejected", Passed: false, Message: "mark_clue_found: clue not found"})
	assert.Contains(t, decision.Checks, DecisionCheck{Code: "roll_missing", Passed: false, Message: "missing roll"})
}

func rawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(value)
	require.NoError(t, err)
	return b
}
