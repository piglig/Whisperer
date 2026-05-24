package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/authoring"
)

func TestLintBundledScenariosSingle(t *testing.T) {
	reports, err := lintBundledScenarios("fog_harbor", false)

	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, "fog_harbor", reports[0].ScenarioID)
	assert.False(t, reports[0].HasErrors())
}

func TestLintBundledScenariosAll(t *testing.T) {
	reports, err := lintBundledScenarios("", true)

	require.NoError(t, err)
	require.NotEmpty(t, reports)
	ids := map[string]bool{}
	for _, report := range reports {
		ids[report.ScenarioID] = true
	}
	assert.True(t, ids["fog_harbor"])
}

func TestScenarioPlaytestAllReportsPass(t *testing.T) {
	adapter, err := authoring.Adapter("fog_harbor")
	require.NoError(t, err)
	reports, err := adapter.RunAllPlaytests(t.Context())

	require.NoError(t, err)
	assert.False(t, authoring.HasFailures(reports))
}

func TestScenarioVerifyPasses(t *testing.T) {
	adapter, err := authoring.Adapter("fog_harbor")
	require.NoError(t, err)
	report, err := adapter.Verify(t.Context())

	require.NoError(t, err)
	assert.True(t, report.Passed)
	assert.False(t, report.ScenarioLint.HasErrors())
	assert.False(t, authoring.HasFailures(report.Playtests))
}

func TestAuthoringAdapterRejectsUnsupportedScenarios(t *testing.T) {
	_, err := authoring.Adapter("other_case")

	require.Error(t, err)
	assert.Contains(t, err.Error(), `unsupported scenario "other_case"`)
}
