package replay

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/agent"
	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
	"github.com/zhuzhenwu/whisperer/internal/orchestrator/sla"
)

func TestBuildDocumentSummarizesTrace(t *testing.T) {
	entries := []orchestrator.TraceEntry{testEntry(1, false), testEntry(2, true)}

	doc := BuildDocument("runs/save/trace.jsonl", entries, map[int][]Mark{
		2: {{Turn: 2, Tags: []string{"误判"}, Note: "judge too strict"}},
	})

	require.Equal(t, SchemaVersion, doc.SchemaVersion)
	assert.Equal(t, "save-1", doc.SaveID)
	assert.Equal(t, 2, doc.Totals.Turns)
	assert.Equal(t, 1, doc.Totals.Blocked)
	assert.Equal(t, 1, doc.Totals.SLAIssues)
	assert.Equal(t, 1, doc.Totals.JudgeIssues)
	assert.Equal(t, 1, doc.Totals.Marked)
	assert.True(t, doc.Turns[1].Blocked)
	assert.Contains(t, doc.Turns[1].BlockReasons[0], "tool rejected")
	assert.Equal(t, "mark_clue_found", doc.Turns[0].Tools[0].Name)
}

func TestRenderHTMLContainsReplayPayload(t *testing.T) {
	doc := BuildDocument("trace.jsonl", []orchestrator.TraceEntry{testEntry(1, false)}, nil)

	html, err := RenderHTML(doc)

	require.NoError(t, err)
	assert.Contains(t, html, "Whisperer Replay")
	assert.Contains(t, html, "replay-data")
	assert.Contains(t, html, "Tool Calls")
	assert.Contains(t, html, "导出标记 JSON")
	assert.Contains(t, html, `"schema_version":"replay.v1"`)
}

func TestDecodeEntriesRejectsEmptyTrace(t *testing.T) {
	_, err := DecodeEntries(strings.NewReader("\n"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no entries")
}

func testEntry(turn int, blocked bool) orchestrator.TraceEntry {
	report := sla.Report{Passed: true}
	checks := []orchestrator.DecisionCheck{{Code: "rules_applied", Passed: true}}
	tool := agent.ToolCall{
		Name:   "mark_clue_found",
		Input:  json.RawMessage(`{"clue_id":"blood_letter"}`),
		Output: json.RawMessage(`{"ok":true}`),
		Iter:   1,
	}
	if blocked {
		report = sla.Report{
			Passed: false,
			Violations: []sla.Violation{{
				Code:    sla.CodeRollMissing,
				Message: "tool rejected the action",
			}},
			JudgeChecks: []sla.JudgeCheck{{
				Kind:    "failure_contradiction",
				Passed:  false,
				Message: "failure was narrated as success",
			}},
		}
		checks = []orchestrator.DecisionCheck{{Code: "tool_rejected", Passed: false, Message: "tool rejected the action"}}
		tool = agent.ToolCall{
			Name:    "roll_skill",
			Input:   json.RawMessage(`{"skill":"spot_hidden"}`),
			Output:  json.RawMessage(`{"error":"tool rejected the action"}`),
			Iter:    1,
			IsError: true,
		}
	}
	return orchestrator.TraceEntry{
		TraceAtMS:  1700000000000,
		SaveID:     "save-1",
		VariantID:  "vance_executes",
		TurnNumber: turn,
		UserInput:  "我查看公告栏",
		Result: orchestrator.TurnResult{
			Narrative: "你发现一封血迹便条。",
			Trace: agent.TurnTrace{
				InputTokens:  10,
				OutputTokens: 20,
				TotalCostUSD: 0.01,
				ToolCalls:    []agent.ToolCall{tool},
			},
			Decision: orchestrator.TurnDecision{
				Intent:       orchestrator.IntentInvestigate,
				StateChanges: []orchestrator.DecisionChange{{Kind: "clue", ID: "blood_letter", Detail: "血迹便条"}},
				Checks:       checks,
			},
			SLAReport: report,
		},
	}
}
