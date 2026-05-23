package fogharbor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunPlaytestMainline(t *testing.T) {
	report, err := RunPlaytest(context.Background(), PlaytestOptions{
		VariantID: "vance_executes",
		Path:      PathMainline,
	})

	require.NoError(t, err)
	assert.True(t, report.Passed, report.Notes)
	assert.Equal(t, "solved", report.EndingID)
	assert.Contains(t, report.FiredTriggers, "culprit_confronted")
	assert.Contains(t, report.StagePath, "confrontation")
	assert.Empty(t, report.MissingKeyClues)
}

func TestRunPlaytestFlee(t *testing.T) {
	report, err := RunPlaytest(context.Background(), PlaytestOptions{Path: PathFlee})

	require.NoError(t, err)
	assert.True(t, report.Passed, report.Notes)
	assert.Equal(t, "flee_with_truth", report.EndingID)
	assert.NotContains(t, report.FiredTriggers, "reef_cave_open")
}

func TestRunPlaytestDismissed(t *testing.T) {
	report, err := RunPlaytest(context.Background(), PlaytestOptions{Path: PathDismissed})

	require.NoError(t, err)
	assert.True(t, report.Passed, report.Notes)
	assert.Equal(t, "dismissed", report.EndingID)
}

func TestRunAllPlaytests(t *testing.T) {
	reports, err := RunAllPlaytests(context.Background())

	require.NoError(t, err)
	require.Len(t, reports, 5)
	assert.False(t, HasFailures(reports))
	seen := map[string]bool{}
	for _, report := range reports {
		seen[report.VariantID+"/"+report.Path] = true
	}
	assert.True(t, seen["vance_executes/mainline"])
	assert.True(t, seen["calvin_directs/mainline"])
	assert.True(t, seen["rourke_runs/mainline"])
}

func TestMarshalJSONReportsShape(t *testing.T) {
	reports := []PlaytestReport{{ScenarioID: "fog_harbor", Path: "mainline", Passed: true}}

	one, err := MarshalJSONReports(reports, false)
	require.NoError(t, err)
	assert.Contains(t, string(one), `"scenario_id"`)
	assert.NotContains(t, string(one), `[`+"\n")

	many, err := MarshalJSONReports(reports, true)
	require.NoError(t, err)
	assert.Contains(t, string(many), `[`+"\n")
}

func TestValidatePath(t *testing.T) {
	path, err := ValidatePath("flee")
	require.NoError(t, err)
	assert.Equal(t, PathFlee, path)

	_, err = ValidatePath("bad")
	assert.Error(t, err)
}
