package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/fogharbor"
)

func TestFogHarborPlaytestAllReportsPass(t *testing.T) {
	reports, err := fogharbor.RunAllPlaytests(t.Context())

	require.NoError(t, err)
	assert.False(t, fogharbor.HasFailures(reports))
}

func TestFogHarborVerifyPasses(t *testing.T) {
	report, err := fogharbor.Verify(t.Context())

	require.NoError(t, err)
	assert.True(t, report.Passed)
	assert.False(t, report.ScenarioLint.HasErrors())
	assert.False(t, fogharbor.HasFailures(report.Playtests))
}
