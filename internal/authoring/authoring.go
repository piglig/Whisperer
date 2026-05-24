package authoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/zhuzhenwu/whisperer/internal/scenario"
)

type PlaytestReport struct {
	ScenarioID      string                `json:"scenario_id"`
	VariantID       string                `json:"variant_id,omitempty"`
	Path            string                `json:"path"`
	Turns           int                   `json:"turns"`
	EndingID        string                `json:"ending_id,omitempty"`
	EndingKind      string                `json:"ending_kind,omitempty"`
	Expected        []string              `json:"expected"`
	Passed          bool                  `json:"passed"`
	FoundKeyClues   []string              `json:"found_key_clues,omitempty"`
	MissingKeyClues []string              `json:"missing_key_clues,omitempty"`
	FiredTriggers   []string              `json:"fired_triggers,omitempty"`
	StagePath       []string              `json:"stage_path,omitempty"`
	BlockedActions  []BlockedActionReport `json:"blocked_actions,omitempty"`
	Notes           []string              `json:"notes,omitempty"`
}

type BlockedActionReport struct {
	Turn               int      `json:"turn"`
	ReasonCode         string   `json:"reason_code,omitempty"`
	PlayerFacingReason string   `json:"player_facing_reason,omitempty"`
	DebugReason        string   `json:"debug_reason,omitempty"`
	RequiredClues      []string `json:"required_clues,omitempty"`
	SuggestedActions   []string `json:"suggested_actions,omitempty"`
}

type PlaytestOptions struct {
	VariantID string
	Path      string
}

type VerifyReport struct {
	ScenarioLint scenario.LintReport `json:"scenario_lint"`
	Playtests    []PlaytestReport    `json:"playtests"`
	Gates        []ContentGateReport `json:"gates"`
	Passed       bool                `json:"passed"`
}

type ContentGateReport struct {
	ID      string   `json:"id"`
	Passed  bool     `json:"passed"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

type ScenarioAdapter interface {
	ID() string
	DisplayName() string
	ValidatePath(path string) (string, error)
	RunPlaytest(ctx context.Context, opts PlaytestOptions) (PlaytestReport, error)
	RunAllPlaytests(ctx context.Context) ([]PlaytestReport, error)
	Verify(ctx context.Context) (VerifyReport, error)
}

type CustomGateFunc func(scn *scenario.Scenario, lint scenario.LintReport, playtests []PlaytestReport) []ContentGateReport

var registry = struct {
	sync.RWMutex
	adapters map[string]ScenarioAdapter
}{adapters: map[string]ScenarioAdapter{}}

func Register(adapter ScenarioAdapter) {
	if adapter == nil || strings.TrimSpace(adapter.ID()) == "" {
		panic("authoring: adapter id is required")
	}
	registry.Lock()
	defer registry.Unlock()
	registry.adapters[adapter.ID()] = adapter
}

func Adapter(id string) (ScenarioAdapter, error) {
	registry.RLock()
	defer registry.RUnlock()
	adapter, ok := registry.adapters[id]
	if !ok {
		return nil, fmt.Errorf("unsupported scenario %q; no authoring adapter registered", id)
	}
	return adapter, nil
}

func MustAdapter(id string) ScenarioAdapter {
	adapter, err := Adapter(id)
	if err != nil {
		panic(err)
	}
	return adapter
}

func HasFailures(reports []PlaytestReport) bool {
	for _, report := range reports {
		if !report.Passed {
			return true
		}
	}
	return false
}

func GatesPassed(gates []ContentGateReport) bool {
	for _, gate := range gates {
		if !gate.Passed {
			return false
		}
	}
	return true
}

func EvaluateGenericGates(scn *scenario.Scenario, lint scenario.LintReport, playtests []PlaytestReport) []ContentGateReport {
	return []ContentGateReport{
		GateNoLintErrors(lint),
		GateMainlineVariants(scn, playtests),
		GateMainlineKeyClues(playtests),
		GateStageCoverage(playtests, []string{scenario.DefaultStage, "investigation", "confrontation"}),
		GateTriggerActivity(playtests, 3),
		GateBlockedActionsExplain(playtests),
	}
}

func VerifyBundledScenario(ctx context.Context, scenarioID string, runAll func(context.Context) ([]PlaytestReport, error), custom CustomGateFunc) (VerifyReport, error) {
	scn, err := scenario.LoadBundled(scenarioID)
	if err != nil {
		return VerifyReport{}, err
	}
	lint := scenario.Lint(scn)
	playtests, err := runAll(ctx)
	if err != nil {
		return VerifyReport{}, err
	}
	gates := EvaluateGenericGates(scn, lint, playtests)
	if custom != nil {
		gates = append(gates, custom(scn, lint, playtests)...)
	}
	return VerifyReport{
		ScenarioLint: lint,
		Playtests:    playtests,
		Gates:        gates,
		Passed:       !lint.HasErrors() && !HasFailures(playtests) && GatesPassed(gates),
	}, nil
}

func GateNoLintErrors(lint scenario.LintReport) ContentGateReport {
	return ContentGateReport{
		ID:      "lint_clean",
		Passed:  !lint.HasErrors(),
		Message: "scenario lint must have zero errors",
	}
}

func GateMainlineVariants(scn *scenario.Scenario, reports []PlaytestReport) ContentGateReport {
	want := map[string]bool{}
	if scn != nil {
		for _, variant := range scn.Variants {
			want[variant.ID] = true
		}
	}
	for _, report := range reports {
		if report.Path == "mainline" && report.Passed {
			delete(want, report.VariantID)
		}
	}
	details := []string{}
	for id := range want {
		details = append(details, "missing passing mainline variant: "+id)
	}
	return ContentGateReport{
		ID:      "mainline_all_variants",
		Passed:  len(details) == 0,
		Message: "every variant must pass a mainline playtest",
		Details: SortedUnique(details),
	}
}

func GateEndingCoverage(reports []PlaytestReport, expected []string) ContentGateReport {
	endings := map[string]bool{}
	for _, report := range reports {
		if report.Passed && report.EndingID != "" {
			endings[report.EndingID] = true
		}
	}
	details := []string{}
	for _, id := range expected {
		if !endings[id] {
			details = append(details, "missing ending: "+id)
		}
	}
	return ContentGateReport{
		ID:      "ending_coverage",
		Passed:  len(details) == 0,
		Message: "verification must cover expected endings: " + strings.Join(expected, ", "),
		Details: details,
	}
}

func GateMainlineKeyClues(reports []PlaytestReport) ContentGateReport {
	details := []string{}
	for _, report := range reports {
		if report.Path != "mainline" {
			continue
		}
		if len(report.MissingKeyClues) > 0 {
			details = append(details, fmt.Sprintf("%s missing key clues: %s",
				NonEmpty(report.VariantID, "base"), strings.Join(report.MissingKeyClues, ", ")))
		}
	}
	return ContentGateReport{
		ID:      "mainline_key_clues",
		Passed:  len(details) == 0,
		Message: "mainline playtests must discover every key clue before ending",
		Details: details,
	}
}

func GateStageCoverage(reports []PlaytestReport, required []string) ContentGateReport {
	details := []string{}
	for _, report := range reports {
		if report.Path != "mainline" {
			continue
		}
		for _, stage := range required {
			if !Contains(report.StagePath, stage) {
				details = append(details, fmt.Sprintf("%s mainline missing stage: %s", NonEmpty(report.VariantID, "base"), stage))
			}
		}
	}
	return ContentGateReport{
		ID:      "stage_coverage",
		Passed:  len(details) == 0,
		Message: "mainline playtests must traverse required story stages",
		Details: details,
	}
}

func GateTriggerActivity(reports []PlaytestReport, minimum int) ContentGateReport {
	details := []string{}
	for _, report := range reports {
		if report.Path != "mainline" {
			continue
		}
		if len(report.FiredTriggers) < minimum {
			details = append(details, fmt.Sprintf("%s mainline fired only %d triggers", NonEmpty(report.VariantID, "base"), len(report.FiredTriggers)))
		}
	}
	return ContentGateReport{
		ID:      "trigger_activity",
		Passed:  len(details) == 0,
		Message: fmt.Sprintf("mainline playtests must exercise at least %d scenario triggers", minimum),
		Details: details,
	}
}

func GateBlockedActionsExplain(reports []PlaytestReport) ContentGateReport {
	details := []string{}
	for _, report := range reports {
		for _, action := range report.BlockedActions {
			if strings.TrimSpace(action.ReasonCode) == "" {
				details = append(details, fmt.Sprintf("%s turn %d missing reason_code", report.Path, action.Turn))
			}
			if strings.TrimSpace(action.PlayerFacingReason) == "" {
				details = append(details, fmt.Sprintf("%s turn %d missing player_facing_reason", report.Path, action.Turn))
			}
			if len(action.SuggestedActions) == 0 && len(action.RequiredClues) == 0 {
				details = append(details, fmt.Sprintf("%s turn %d missing suggested_actions or required_clues", report.Path, action.Turn))
			}
		}
	}
	return ContentGateReport{
		ID:      "blocked_action_clarity",
		Passed:  len(details) == 0,
		Message: "blocked actions must explain the reason and recovery path",
		Details: details,
	}
}

func FormatPlaytestReport(displayName string, report PlaytestReport) string {
	lines := []string{
		displayName + " Playtest Report",
		"",
		"variant: " + NonEmpty(report.VariantID, "base"),
		"path: " + report.Path,
		fmt.Sprintf("turns: %d", report.Turns),
		"ending: " + NonEmpty(report.EndingID, "<none>"),
		"expected: " + strings.Join(report.Expected, ", "),
		fmt.Sprintf("result: %s", PassLabel(report.Passed)),
	}
	if len(report.FoundKeyClues) > 0 || len(report.MissingKeyClues) > 0 {
		lines = append(lines, "key clues: "+fmt.Sprintf("%d found / %d missing", len(report.FoundKeyClues), len(report.MissingKeyClues)))
	}
	if len(report.StagePath) > 0 {
		lines = append(lines, "stage path: "+strings.Join(report.StagePath, " -> "))
	}
	if len(report.FiredTriggers) > 0 {
		lines = append(lines, "fired: "+strings.Join(report.FiredTriggers, ", "))
	}
	if len(report.Notes) > 0 {
		lines = append(lines, "", "notes:")
		for _, note := range report.Notes {
			lines = append(lines, "- "+note)
		}
	}
	return strings.Join(lines, "\n")
}

func FormatPlaytestReports(displayName string, reports []PlaytestReport) string {
	if len(reports) == 1 {
		return FormatPlaytestReport(displayName, reports[0])
	}
	lines := []string{
		displayName + " Playtest Report",
		"",
		fmt.Sprintf("runs: %d", len(reports)),
	}
	for _, report := range reports {
		lines = append(lines, fmt.Sprintf("- %s / %s: %s (%s, %d turns)",
			NonEmpty(report.VariantID, "base"), report.Path, PassLabel(report.Passed), NonEmpty(report.EndingID, "<none>"), report.Turns))
	}
	return strings.Join(lines, "\n")
}

func FormatVerifyReport(displayName string, report VerifyReport) string {
	status := "pass"
	if !report.Passed {
		status = "fail"
	}
	warnings, infos := LintFindingCounts(report.ScenarioLint)
	lines := []string{
		displayName + " Verify Report",
		"",
		"status: " + status,
		fmt.Sprintf("lint: %d warning · %d info", warnings, infos),
		fmt.Sprintf("playtests: %d run", len(report.Playtests)),
	}
	if len(report.Gates) > 0 {
		lines = append(lines, "content gates:")
		for _, gate := range report.Gates {
			lines = append(lines, fmt.Sprintf("- %s: %s — %s", gate.ID, PassLabel(gate.Passed), gate.Message))
			for _, detail := range gate.Details {
				lines = append(lines, "    "+detail)
			}
		}
	}
	for _, playtest := range report.Playtests {
		lines = append(lines, fmt.Sprintf("- %s / %s: %s (%s, %d turns)",
			NonEmpty(playtest.VariantID, "base"),
			playtest.Path,
			PassLabel(playtest.Passed),
			NonEmpty(playtest.EndingID, "<none>"),
			playtest.Turns,
		))
	}
	if report.ScenarioLint.HasErrors() {
		lines = append(lines, "", "lint errors:")
		for _, line := range LintErrorLines(report.ScenarioLint) {
			lines = append(lines, "- "+line)
		}
	}
	return strings.Join(lines, "\n")
}

func MarshalPlaytestJSON(reports []PlaytestReport, forceArray bool) ([]byte, error) {
	if len(reports) == 1 && !forceArray {
		return json.MarshalIndent(reports[0], "", "  ")
	}
	return json.MarshalIndent(reports, "", "  ")
}

func MarshalVerifyJSON(report VerifyReport) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

func LintFindingCounts(report scenario.LintReport) (warnings, infos int) {
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

func LintErrorLines(report scenario.LintReport) []string {
	lines := []string{}
	for _, finding := range report.Findings {
		if finding.Severity == scenario.LintError {
			lines = append(lines, finding.Message)
		}
	}
	for _, variant := range report.Variants {
		for _, finding := range variant.Findings {
			if finding.Severity == scenario.LintError {
				lines = append(lines, fmt.Sprintf("%s: %s", variant.ID, finding.Message))
			}
		}
	}
	return lines
}

func PassLabel(pass bool) string {
	if pass {
		return "pass"
	}
	return "fail"
}

func NonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func Contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func SortedUnique(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func ValidatePathIn(path string, allowed ...string) (string, error) {
	for _, value := range allowed {
		if path == value {
			return path, nil
		}
	}
	return "", errors.New("path must be one of: " + strings.Join(allowed, ", "))
}
