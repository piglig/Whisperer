package fogharbor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyPassesFogHarborHealthGate(t *testing.T) {
	report, err := Verify(t.Context())

	require.NoError(t, err)
	assert.True(t, report.Passed)
	assert.Equal(t, "fog_harbor", report.ScenarioLint.ScenarioID)
	require.NotEmpty(t, report.Playtests)
	assert.False(t, HasFailures(report.Playtests))
}

func TestFormatVerifyReport(t *testing.T) {
	report, err := Verify(t.Context())
	require.NoError(t, err)

	out := FormatVerifyReport(report)

	assert.Contains(t, out, "Fog Harbor Verify Report")
	assert.Contains(t, out, "status: pass")
	assert.Contains(t, out, "playtests:")
	assert.Contains(t, out, "vance_executes / mainline")
}

func TestMarshalVerifyJSON(t *testing.T) {
	report, err := Verify(t.Context())
	require.NoError(t, err)

	data, err := MarshalVerifyJSON(report)

	require.NoError(t, err)
	assert.True(t, strings.Contains(string(data), "\n  "))
	var decoded VerifyReport
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.True(t, decoded.Passed)
	assert.Equal(t, "fog_harbor", decoded.ScenarioLint.ScenarioID)
}
