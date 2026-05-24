package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/agent"
	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
	"github.com/zhuzhenwu/whisperer/internal/orchestrator/sla"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
)

func TestRenderReplayHumanReadable(t *testing.T) {
	path := writeReplayFixture(t, []orchestrator.TraceEntry{replayFixtureEntry(1), replayFixtureEntry(2)})

	out, err := RenderReplay(replayOptions{Path: path, ShowTools: true})

	require.NoError(t, err)
	assert.Contains(t, out, "Whisperer Replay")
	assert.Contains(t, out, "turns: 2 (1 -> 2)")
	assert.Contains(t, out, "=== Turn 1 ===")
	assert.Contains(t, out, "[player]")
	assert.Contains(t, out, "我查看公告栏")
	assert.Contains(t, out, "[gm]")
	assert.Contains(t, out, "你发现一封血迹便条")
	assert.Contains(t, out, "[recap]")
	assert.Contains(t, out, "intent: investigate")
	assert.Contains(t, out, "clue blood_letter")
	assert.Contains(t, out, "[tools]")
	assert.Contains(t, out, "mark_clue_found [ok]")
	assert.Contains(t, out, "[sla]")
	assert.Contains(t, out, "passed: true")
	assert.Contains(t, out, "[judge]")
	assert.Contains(t, out, "npc_consistency: pass target=vance")
	assert.Contains(t, out, "[ending]")
	assert.Contains(t, out, "success / solved")
}

func TestRenderReplayFiltersTurn(t *testing.T) {
	path := writeReplayFixture(t, []orchestrator.TraceEntry{replayFixtureEntry(1), replayFixtureEntry(2)})

	out, err := RenderReplay(replayOptions{Path: path, Turn: 2, ShowTools: false})

	require.NoError(t, err)
	assert.NotContains(t, out, "=== Turn 1 ===")
	assert.Contains(t, out, "=== Turn 2 ===")
	assert.NotContains(t, out, "[tools]")
}

func TestRenderReplayTurnNotFound(t *testing.T) {
	path := writeReplayFixture(t, []orchestrator.TraceEntry{replayFixtureEntry(1)})

	_, err := RenderReplay(replayOptions{Path: path, Turn: 99})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "turn 99")
}

func TestResolveReplayPathPicksNewestJSONL(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")
	newPath := filepath.Join(dir, "nested", "new.jsonl")
	require.NoError(t, os.WriteFile(oldPath, []byte("{}\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Dir(newPath), 0o755))
	require.NoError(t, os.WriteFile(newPath, []byte("{}\n"), 0o644))
	oldTime := time.Now().Add(-time.Hour)
	newTime := time.Now()
	require.NoError(t, os.Chtimes(oldPath, oldTime, oldTime))
	require.NoError(t, os.Chtimes(newPath, newTime, newTime))

	got, err := resolveReplayPath(dir)

	require.NoError(t, err)
	assert.Equal(t, newPath, got)
}

func writeReplayFixture(t *testing.T, entries []orchestrator.TraceEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, entry := range entries {
		require.NoError(t, enc.Encode(entry))
	}
	return path
}

func replayFixtureEntry(turn int) orchestrator.TraceEntry {
	entry := orchestrator.TraceEntry{
		TraceAtMS:  int64(1700000000000 + turn),
		SaveID:     "save-1",
		VariantID:  "vance_executes",
		TurnNumber: turn,
		UserInput:  "我查看公告栏",
		Result: orchestrator.TurnResult{
			Narrative: "你发现一封血迹便条。",
			Trace: agent.TurnTrace{
				InputTokens:  10,
				OutputTokens: 20,
				TotalCostUSD: 0.001,
				ToolCalls: []agent.ToolCall{{
					Name:   "mark_clue_found",
					Input:  json.RawMessage(`{"clue_id":"blood_letter","location_id":"harbor"}`),
					Output: json.RawMessage(`{"ok":true}`),
					Iter:   1,
				}},
			},
			Decision: orchestrator.TurnDecision{
				Intent: orchestrator.IntentInvestigate,
				StateChanges: []orchestrator.DecisionChange{{
					Kind:   "clue",
					ID:     "blood_letter",
					Detail: "血迹便条",
				}},
				Checks: []orchestrator.DecisionCheck{{Code: "rules_applied", Passed: true}},
			},
			SLAReport: sla.Report{
				Passed: true,
				JudgeChecks: []sla.JudgeCheck{{
					Kind:   "npc_consistency",
					Target: "vance",
					Passed: true,
				}},
			},
		},
	}
	if turn == 2 {
		entry.Result.Ending = &scenario.Ending{ID: "solved", Kind: "success", Description: "真相公开。"}
		entry.Result.Report = &scenario.CaseReport{
			Ending:         *entry.Result.Ending,
			EvidenceStatus: "关键证据完整",
			CulpritName:    "范斯医生",
			FoundKeyClues:  []scenario.ReportClue{{ID: "blood_letter", Description: "血迹便条"}},
		}
	}
	return entry
}

func TestRenderReplayExportsScript(t *testing.T) {
	path := writeReplayFixture(t, []orchestrator.TraceEntry{replayFixtureEntry(1), replayFixtureEntry(2)})
	outPath := filepath.Join(t.TempDir(), "script.txt")

	out, err := RenderReplay(replayOptions{Path: path, ExportScript: outPath})

	require.NoError(t, err)
	assert.Contains(t, out, "playtest script exported")
	raw, err := os.ReadFile(outPath)
	require.NoError(t, err)
	body := string(raw)
	assert.Contains(t, body, "# Turn 1")
	assert.Contains(t, body, "我查看公告栏")
	assert.Contains(t, body, "# Turn 2")
}

func TestRenderReplayExportsHTMLViewer(t *testing.T) {
	path := writeReplayFixture(t, []orchestrator.TraceEntry{replayFixtureEntry(1), replayFixtureEntry(2)})
	outPath := filepath.Join(t.TempDir(), "viewer.html")

	out, err := RenderReplay(replayOptions{Path: path, HTML: outPath})

	require.NoError(t, err)
	assert.Contains(t, out, "replay viewer exported")
	raw, err := os.ReadFile(outPath)
	require.NoError(t, err)
	body := string(raw)
	assert.Contains(t, body, "Whisperer Replay")
	assert.Contains(t, body, `"schema_version":"replay.v1"`)
	assert.Contains(t, body, "为什么被拦截")
	assert.Contains(t, body, "复制 playtest script")
}

func TestRenderReplayMarksTurn(t *testing.T) {
	path := writeReplayFixture(t, []orchestrator.TraceEntry{replayFixtureEntry(1)})

	out, err := RenderReplay(replayOptions{
		Path: path,
		Turn: 1,
		Mark: "bad,misjudge",
		Note: "NPC answer contradicted known clue",
	})
	require.NoError(t, err)
	assert.Contains(t, out, "turn 1 marked")

	out, err = RenderReplay(replayOptions{Path: path})
	require.NoError(t, err)
	assert.Contains(t, out, "[marks]")
	assert.Contains(t, out, "bad, misjudge")
	assert.Contains(t, out, "NPC answer contradicted known clue")
}

func TestRenderReplayMarkRequiresTurn(t *testing.T) {
	path := writeReplayFixture(t, []orchestrator.TraceEntry{replayFixtureEntry(1)})

	_, err := RenderReplay(replayOptions{Path: path, Mark: "bad"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--turn")
}
