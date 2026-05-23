package fogharbor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zhuzhenwu/whisperer/internal/scenario"
)

type VerifyReport struct {
	ScenarioLint scenario.LintReport `json:"scenario_lint"`
	Playtests    []PlaytestReport    `json:"playtests"`
	Passed       bool                `json:"passed"`
}

func Verify(ctx context.Context) (VerifyReport, error) {
	scn, err := scenario.LoadBundled("fog_harbor")
	if err != nil {
		return VerifyReport{}, err
	}
	lint := scenario.Lint(scn)
	playtests, err := RunAllPlaytests(ctx)
	if err != nil {
		return VerifyReport{}, err
	}
	return VerifyReport{
		ScenarioLint: lint,
		Playtests:    playtests,
		Passed:       !lint.HasErrors() && !HasFailures(playtests),
	}, nil
}

func FormatVerifyReport(report VerifyReport) string {
	status := "pass"
	if !report.Passed {
		status = "fail"
	}
	warnings, infos := lintFindingCounts(report.ScenarioLint)
	lines := []string{
		"Fog Harbor Verify Report",
		"",
		"status: " + status,
		fmt.Sprintf("lint: %d warning · %d info", warnings, infos),
		fmt.Sprintf("playtests: %d run", len(report.Playtests)),
	}
	for _, playtest := range report.Playtests {
		lines = append(lines, fmt.Sprintf("- %s / %s: %s (%s, %d turns)",
			nonEmpty(playtest.VariantID, "base"),
			playtest.Path,
			passLabel(playtest.Passed),
			nonEmpty(playtest.EndingID, "<none>"),
			playtest.Turns,
		))
	}
	if report.ScenarioLint.HasErrors() {
		lines = append(lines, "", "lint errors:")
		appendLintErrors(&lines, report.ScenarioLint)
	}
	return strings.Join(lines, "\n")
}

func MarshalVerifyJSON(report VerifyReport) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

func lintFindingCounts(report scenario.LintReport) (warnings, infos int) {
	for _, finding := range report.Findings {
		switch finding.Severity {
		case scenario.LintWarning:
			warnings++
		case scenario.LintInfo:
			infos++
		}
	}
	for _, variant := range report.Variants {
		for _, finding := range variant.Findings {
			switch finding.Severity {
			case scenario.LintWarning:
				warnings++
			case scenario.LintInfo:
				infos++
			}
		}
	}
	return warnings, infos
}

func appendLintErrors(lines *[]string, report scenario.LintReport) {
	for _, finding := range report.Findings {
		if finding.Severity == scenario.LintError {
			*lines = append(*lines, "- "+finding.Message)
		}
	}
	for _, variant := range report.Variants {
		for _, finding := range variant.Findings {
			if finding.Severity == scenario.LintError {
				*lines = append(*lines, fmt.Sprintf("- %s: %s", variant.ID, finding.Message))
			}
		}
	}
}
