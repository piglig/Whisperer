package authoring

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/scenario"
)

func TestGenericGatesFailOnCoverageGaps(t *testing.T) {
	scn := &scenario.Scenario{Variants: []scenario.Variant{{ID: "vance_executes"}, {ID: "rourke_runs"}}}
	reports := []PlaytestReport{{
		VariantID:       "vance_executes",
		Path:            "mainline",
		Passed:          true,
		EndingID:        "solved",
		StagePath:       []string{scenario.DefaultStage},
		MissingKeyClues: []string{"reef_carvings"},
		FiredTriggers:   []string{"one"},
	}}

	gates := EvaluateGenericGates(scn, scenario.LintReport{}, reports)
	gates = append(gates, GateEndingCoverage(reports, []string{"solved", "dismissed"}))

	require.False(t, GatesPassed(gates))
	failed := map[string]bool{}
	for _, gate := range gates {
		if !gate.Passed {
			failed[gate.ID] = true
		}
	}
	assert.True(t, failed["mainline_all_variants"])
	assert.True(t, failed["mainline_key_clues"])
	assert.True(t, failed["stage_coverage"])
	assert.True(t, failed["trigger_activity"])
	assert.True(t, failed["ending_coverage"])
}

func TestGateBlockedActionsExplainFailsUnclearBlocks(t *testing.T) {
	gate := GateBlockedActionsExplain([]PlaytestReport{{
		Path: "mainline",
		BlockedActions: []BlockedActionReport{{
			Turn:               3,
			PlayerFacingReason: "现在不能这么做。",
		}},
	}})

	require.False(t, gate.Passed)
	assert.Contains(t, gate.Details[0], "reason_code")
}

func TestRegistryRejectsMissingAdapter(t *testing.T) {
	_, err := Adapter("missing")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no authoring adapter")
}

func TestMarshalPlaytestJSONSingleAndArray(t *testing.T) {
	reports := []PlaytestReport{{ScenarioID: "case", Path: "mainline", Passed: true}}

	one, err := MarshalPlaytestJSON(reports, false)
	require.NoError(t, err)
	assert.Contains(t, string(one), `"scenario_id"`)
	assert.NotContains(t, string(one), `[`+"\n")

	many, err := MarshalPlaytestJSON(reports, true)
	require.NoError(t, err)
	assert.Contains(t, string(many), `[`+"\n")
}
