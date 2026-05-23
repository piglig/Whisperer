package scenario

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLintDetectsPlayabilityFindings(t *testing.T) {
	scn, err := Parse([]byte(`
id: x
title: Test Case
objectives:
  - {stage: opening, title: Start}
locations:
  - {id: room, name: Room, description: Empty}
npcs: []
clues:
  - {id: loose, description: No path, tier: 1}
  - {id: key, description: Key clue, tier: 1, location: room}
items: []
start: {location: room}
key_clues: [loose]
triggers: []
endings:
  - id: solved
    kind: success
    when: {clue_found: key}
    description: solved
`))
	require.NoError(t, err)

	report := Lint(scn)

	assert.False(t, report.HasErrors())
	assertFinding(t, report.Findings, "clue_unreachable", "loose")
	assertFinding(t, report.Findings, "key_clue_unreachable", "loose")
	assertFinding(t, report.Findings, "ending_no_failure", "")
	assertFinding(t, report.Findings, "tier_low_redundancy", "tier:1")
	require.Len(t, report.Stages, 1)
	assert.Equal(t, "opening", report.Stages[0].Stage)
	assert.Equal(t, 0, report.Stages[0].LocationLeads)
	assert.Contains(t, report.Stages[0].ReachableClues, "key")

	text := FormatLintReport(report)
	assert.Contains(t, text, "Test Case Playability Report")
	assert.Contains(t, text, "clue_unreachable loose")
}

func TestLintIncludesVariantReports(t *testing.T) {
	scn, err := Parse([]byte(`
id: x
title: Variant Case
objectives:
  - {stage: opening, title: Start}
locations:
  - {id: room, name: Room, description: Empty, leads: [Look around]}
  - {id: hall, name: Hall, description: Empty}
npcs: []
clues:
  - {id: key, description: Key clue, tier: 1, location: room}
start: {location: room}
key_clues: [key]
triggers: []
endings:
  - id: solved
    kind: success
    when: {clue_found: key}
    description: solved
variants:
  - id: moved
    clue_overrides:
      key: {location: hall}
`))
	require.NoError(t, err)

	report := Lint(scn)

	require.Len(t, report.Variants, 1)
	assert.Equal(t, "moved", report.Variants[0].ID)
	assert.NotContains(t, strings.Join(findingCodes(report.Variants[0].Findings), ","), "validate_failed")
}

func TestLintValidateFailureBecomesErrorFinding(t *testing.T) {
	report := Lint(&Scenario{ID: "bad"})

	assert.True(t, report.HasErrors())
	assertFinding(t, report.Findings, "validate_failed", "")
}

func TestFormatLintReportsMultiple(t *testing.T) {
	reports := []LintReport{
		{ScenarioID: "a", Title: "A", Locations: 1, NPCs: 2, Clues: 3},
		{
			ScenarioID: "b",
			Title:      "B",
			Locations:  1,
			Findings: []LintFinding{{
				Severity: LintWarning,
				Code:     "stage_no_actions",
				Subject:  "opening",
				Message:  "empty",
			}},
		},
	}

	out := FormatLintReports(reports)

	assert.Contains(t, out, "Bundled Scenario Playability Report")
	assert.Contains(t, out, "Scenarios: 2")
	assert.Contains(t, out, "## A (a)")
	assert.Contains(t, out, "status: warning")
	assert.False(t, HasLintErrors(reports))
}

func TestFormatLintReportsSingleUsesBundleShape(t *testing.T) {
	out := FormatLintReports([]LintReport{{ScenarioID: "a", Title: "A"}})

	assert.Contains(t, out, "Bundled Scenario Playability Report")
	assert.Contains(t, out, "Scenarios: 1")
	assert.Contains(t, out, "## A (a)")
}

func assertFinding(t *testing.T, findings []LintFinding, code, subject string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Code == code && (subject == "" || finding.Subject == subject) {
			return
		}
	}
	t.Fatalf("finding %s/%s not found in %#v", code, subject, findings)
}

func findingCodes(findings []LintFinding) []string {
	out := make([]string, len(findings))
	for i, finding := range findings {
		out[i] = finding.Code
	}
	return out
}
