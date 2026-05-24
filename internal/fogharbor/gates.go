package fogharbor

import (
	"github.com/zhuzhenwu/whisperer/internal/authoring"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
)

func customGates(_ *scenario.Scenario, _ scenario.LintReport, playtests []authoring.PlaytestReport) []authoring.ContentGateReport {
	return []authoring.ContentGateReport{
		authoring.GateEndingCoverage(playtests, []string{"solved", "flee_with_truth", "dismissed"}),
	}
}
