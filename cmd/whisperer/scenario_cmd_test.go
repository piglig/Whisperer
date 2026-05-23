package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
