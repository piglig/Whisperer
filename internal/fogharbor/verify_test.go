package fogharbor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/authoring"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
)

func TestAdapterVerifyPassesFogHarborHealthGate(t *testing.T) {
	report, err := (Adapter{}).Verify(t.Context())

	require.NoError(t, err)
	assert.True(t, report.Passed)
	assert.Equal(t, "fog_harbor", report.ScenarioLint.ScenarioID)
	require.NotEmpty(t, report.Playtests)
	require.NotEmpty(t, report.Gates)
	assert.False(t, authoring.HasFailures(report.Playtests))
	for _, gate := range report.Gates {
		assert.True(t, gate.Passed, gate.ID)
	}
}

func TestAdapterVerifyFormatsReport(t *testing.T) {
	report, err := (Adapter{}).Verify(t.Context())
	require.NoError(t, err)

	out := authoring.FormatVerifyReport("Fog Harbor", report)

	assert.Contains(t, out, "Fog Harbor Verify Report")
	assert.Contains(t, out, "status: pass")
	assert.Contains(t, out, "content gates:")
	assert.Contains(t, out, "mainline_key_clues")
	assert.Contains(t, out, "playtests:")
	assert.Contains(t, out, "vance_executes / mainline")
}

func TestAdapterVerifyMarshalsJSON(t *testing.T) {
	report, err := (Adapter{}).Verify(t.Context())
	require.NoError(t, err)

	data, err := authoring.MarshalVerifyJSON(report)

	require.NoError(t, err)
	assert.True(t, strings.Contains(string(data), "\n  "))
	var decoded authoring.VerifyReport
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.True(t, decoded.Passed)
	assert.Equal(t, "fog_harbor", decoded.ScenarioLint.ScenarioID)
	assert.NotEmpty(t, decoded.Gates)
}

func TestAdapterCustomGatesFailMissingCoverage(t *testing.T) {
	report := authoring.PlaytestReport{
		VariantID:       "vance_executes",
		Path:            string(pathMainline),
		Passed:          true,
		EndingID:        "solved",
		StagePath:       []string{scenario.DefaultStage},
		MissingKeyClues: []string{"reef_carvings"},
		FiredTriggers:   []string{"one"},
	}
	scn := &scenario.Scenario{Variants: []scenario.Variant{{ID: "vance_executes"}, {ID: "rourke_runs"}}}

	gates := append(authoring.EvaluateGenericGates(scn, scenario.LintReport{}, []authoring.PlaytestReport{report}), customGates(scn, scenario.LintReport{}, []authoring.PlaytestReport{report})...)

	assert.False(t, authoring.GatesPassed(gates))
	ids := []string{}
	for _, gate := range gates {
		if !gate.Passed {
			ids = append(ids, gate.ID)
		}
	}
	assert.Contains(t, ids, "mainline_all_variants")
	assert.Contains(t, ids, "ending_coverage")
	assert.Contains(t, ids, "mainline_key_clues")
	assert.Contains(t, ids, "stage_coverage")
	assert.Contains(t, ids, "trigger_activity")
}
