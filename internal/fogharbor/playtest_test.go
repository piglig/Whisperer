package fogharbor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/authoring"
)

func TestAdapterPlaytestMainline(t *testing.T) {
	report, err := runPlaytest(context.Background(), authoring.PlaytestOptions{
		VariantID: "vance_executes",
		Path:      string(pathMainline),
	})

	require.NoError(t, err)
	assert.True(t, report.Passed, report.Notes)
	assert.Equal(t, "solved", report.EndingID)
	assert.Contains(t, report.FiredTriggers, "culprit_confronted")
	assert.Contains(t, report.StagePath, "confrontation")
	assert.Empty(t, report.MissingKeyClues)
}

func TestAdapterPlaytestFlee(t *testing.T) {
	report, err := runPlaytest(context.Background(), authoring.PlaytestOptions{Path: string(pathFlee)})

	require.NoError(t, err)
	assert.True(t, report.Passed, report.Notes)
	assert.Equal(t, "flee_with_truth", report.EndingID)
	assert.NotContains(t, report.FiredTriggers, "reef_cave_open")
}

func TestAdapterPlaytestDismissed(t *testing.T) {
	report, err := runPlaytest(context.Background(), authoring.PlaytestOptions{Path: string(pathDismissed)})

	require.NoError(t, err)
	assert.True(t, report.Passed, report.Notes)
	assert.Equal(t, "dismissed", report.EndingID)
}

func TestAdapterPlaytestSuite(t *testing.T) {
	reports, err := runAllPlaytests(context.Background())

	require.NoError(t, err)
	require.Len(t, reports, 5)
	assert.False(t, authoring.HasFailures(reports))
	seen := map[string]bool{}
	for _, report := range reports {
		seen[report.VariantID+"/"+report.Path] = true
	}
	assert.True(t, seen["vance_executes/mainline"])
	assert.True(t, seen["calvin_directs/mainline"])
	assert.True(t, seen["rourke_runs/mainline"])
}

func TestMarshalJSONReportsShape(t *testing.T) {
	reports := []authoring.PlaytestReport{{ScenarioID: "fog_harbor", Path: "mainline", Passed: true}}

	one, err := authoring.MarshalPlaytestJSON(reports, false)
	require.NoError(t, err)
	assert.Contains(t, string(one), `"scenario_id"`)
	assert.NotContains(t, string(one), `[`+"\n")

	many, err := authoring.MarshalPlaytestJSON(reports, true)
	require.NoError(t, err)
	assert.Contains(t, string(many), `[`+"\n")
}

func TestAdapterValidatePath(t *testing.T) {
	path, err := validatePath("flee")
	require.NoError(t, err)
	assert.Equal(t, pathFlee, path)

	_, err = validatePath("bad")
	assert.Error(t, err)
}
