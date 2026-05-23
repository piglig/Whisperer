package scenario

import (
	"fmt"
	"sort"
	"strings"
)

type LintSeverity string

const (
	LintError   LintSeverity = "error"
	LintWarning LintSeverity = "warning"
	LintInfo    LintSeverity = "info"
)

type LintFinding struct {
	Severity LintSeverity `json:"severity"`
	Code     string       `json:"code"`
	Subject  string       `json:"subject,omitempty"`
	Message  string       `json:"message"`
}

type LintStageReport struct {
	Stage           string   `json:"stage"`
	Title           string   `json:"title,omitempty"`
	LocationLeads   int      `json:"location_leads"`
	DialogueOptions int      `json:"dialogue_options"`
	ItemActions     int      `json:"item_actions"`
	ReachableClues  []string `json:"reachable_clues,omitempty"`
}

type LintVariantReport struct {
	ID       string        `json:"id"`
	Title    string        `json:"title"`
	Findings []LintFinding `json:"findings,omitempty"`
}

type LintReport struct {
	ScenarioID string              `json:"scenario_id"`
	Title      string              `json:"title"`
	Locations  int                 `json:"locations"`
	NPCs       int                 `json:"npcs"`
	Clues      int                 `json:"clues"`
	Items      int                 `json:"items"`
	Triggers   int                 `json:"triggers"`
	Endings    int                 `json:"endings"`
	Variants   []LintVariantReport `json:"variants,omitempty"`
	Stages     []LintStageReport   `json:"stages,omitempty"`
	Findings   []LintFinding       `json:"findings,omitempty"`
}

func HasLintErrors(reports []LintReport) bool {
	for _, report := range reports {
		if report.HasErrors() {
			return true
		}
	}
	return false
}

func (r LintReport) HasErrors() bool {
	for _, finding := range r.Findings {
		if finding.Severity == LintError {
			return true
		}
	}
	for _, variant := range r.Variants {
		for _, finding := range variant.Findings {
			if finding.Severity == LintError {
				return true
			}
		}
	}
	return false
}

func Lint(s *Scenario) LintReport {
	return lintScenario(s, true)
}

func lintScenario(s *Scenario, includeVariants bool) LintReport {
	if s == nil {
		return LintReport{
			Findings: []LintFinding{{
				Severity: LintError,
				Code:     "scenario_nil",
				Message:  "scenario is nil",
			}},
		}
	}
	report := LintReport{
		ScenarioID: s.ID,
		Title:      s.Title,
		Locations:  len(s.Locations),
		NPCs:       len(s.NPCs),
		Clues:      len(s.Clues),
		Items:      len(s.Items),
		Triggers:   len(s.Triggers),
		Endings:    len(s.Endings),
	}
	if err := Validate(s); err != nil {
		report.Findings = append(report.Findings, LintFinding{
			Severity: LintError,
			Code:     "validate_failed",
			Message:  err.Error(),
		})
		return report
	}

	cluePaths := clueAcquisitionPaths(s)
	report.Stages = stageReports(s, cluePaths)
	report.Findings = append(report.Findings, lintClueReachability(s, cluePaths)...)
	report.Findings = append(report.Findings, lintStageActionCoverage(report.Stages)...)
	report.Findings = append(report.Findings, lintKeyClues(s, cluePaths)...)
	report.Findings = append(report.Findings, lintTierRedundancy(s, cluePaths)...)
	report.Findings = append(report.Findings, lintEndingCoverage(s)...)

	if includeVariants {
		for i := range s.Variants {
			eff := MergeVariant(s, &s.Variants[i])
			vr := LintVariantReport{ID: s.Variants[i].ID, Title: eff.Title}
			vreport := lintScenario(eff, false)
			vr.Findings = vreport.Findings
			report.Variants = append(report.Variants, vr)
		}
	}
	sortFindings(report.Findings)
	for i := range report.Variants {
		sortFindings(report.Variants[i].Findings)
	}
	return report
}

func clueAcquisitionPaths(s *Scenario) map[string][]string {
	paths := map[string][]string{}
	for _, clue := range s.Clues {
		if clue.Location != "" {
			paths[clue.ID] = append(paths[clue.ID], "location:"+clue.Location)
		}
		if clue.Source != "" {
			paths[clue.ID] = append(paths[clue.ID], "npc:"+clue.Source)
		}
	}
	for _, trigger := range s.Triggers {
		for _, action := range trigger.Then {
			if action.MarkClueFound != nil {
				paths[action.MarkClueFound.ClueID] = append(paths[action.MarkClueFound.ClueID], "trigger:"+trigger.ID)
			}
		}
	}
	for id := range paths {
		paths[id] = sortedUnique(paths[id])
	}
	return paths
}

func stageReports(s *Scenario, cluePaths map[string][]string) []LintStageReport {
	stages := s.Objectives
	if len(stages) == 0 {
		stages = []Objective{{Stage: DefaultStage}}
	}
	out := make([]LintStageReport, 0, len(stages))
	for _, objective := range stages {
		stage := NormalizeStage(s, objective.Stage)
		if stage == "" {
			stage = objective.Stage
		}
		report := LintStageReport{Stage: stage, Title: objective.Title}
		for _, loc := range s.Locations {
			report.LocationLeads += nonEmptyStringCount(loc.Leads)
			for _, clue := range s.Clues {
				if clue.Location == loc.ID {
					report.ReachableClues = append(report.ReachableClues, clue.ID)
				}
			}
		}
		for _, npc := range s.NPCs {
			for _, opt := range DialogueOptionsFor(npc, stage, nil) {
				report.DialogueOptions++
				for _, clue := range s.Clues {
					if clue.Source == npc.ID {
						report.ReachableClues = append(report.ReachableClues, clue.ID)
					}
				}
				_ = opt
			}
		}
		for _, item := range s.Items {
			for _, action := range item.Actions {
				if itemActionStageAllowed(action, stage) {
					report.ItemActions++
				}
			}
		}
		for clueID, paths := range cluePaths {
			for _, path := range paths {
				if strings.HasPrefix(path, "trigger:") {
					report.ReachableClues = append(report.ReachableClues, clueID)
					break
				}
			}
		}
		report.ReachableClues = sortedUnique(report.ReachableClues)
		out = append(out, report)
	}
	return out
}

func lintClueReachability(s *Scenario, cluePaths map[string][]string) []LintFinding {
	var findings []LintFinding
	for _, clue := range s.Clues {
		if len(cluePaths[clue.ID]) == 0 {
			findings = append(findings, LintFinding{
				Severity: LintWarning,
				Code:     "clue_unreachable",
				Subject:  clue.ID,
				Message:  fmt.Sprintf("clue %q has no location, npc source, or trigger acquisition path", clue.ID),
			})
		}
	}
	return findings
}

func lintStageActionCoverage(stages []LintStageReport) []LintFinding {
	var findings []LintFinding
	for _, stage := range stages {
		total := stage.LocationLeads + stage.DialogueOptions + stage.ItemActions
		if total == 0 {
			findings = append(findings, LintFinding{
				Severity: LintWarning,
				Code:     "stage_no_actions",
				Subject:  stage.Stage,
				Message:  fmt.Sprintf("stage %q has no location leads, dialogue options, or item actions", stage.Stage),
			})
		}
	}
	return findings
}

func lintKeyClues(s *Scenario, cluePaths map[string][]string) []LintFinding {
	var findings []LintFinding
	for _, clueID := range s.KeyClues {
		if len(cluePaths[clueID]) == 0 {
			findings = append(findings, LintFinding{
				Severity: LintWarning,
				Code:     "key_clue_unreachable",
				Subject:  clueID,
				Message:  fmt.Sprintf("key clue %q has no acquisition path", clueID),
			})
		}
	}
	return findings
}

func lintTierRedundancy(s *Scenario, cluePaths map[string][]string) []LintFinding {
	tierSources := map[int]map[string]bool{}
	for _, clue := range s.Clues {
		if clue.Tier <= 0 {
			continue
		}
		if tierSources[clue.Tier] == nil {
			tierSources[clue.Tier] = map[string]bool{}
		}
		for _, path := range cluePaths[clue.ID] {
			tierSources[clue.Tier][path] = true
		}
	}
	var findings []LintFinding
	for tier, sources := range tierSources {
		if len(sources) < 3 {
			findings = append(findings, LintFinding{
				Severity: LintInfo,
				Code:     "tier_low_redundancy",
				Subject:  fmt.Sprintf("tier:%d", tier),
				Message:  fmt.Sprintf("tier %d has %d independent clue paths; target is at least 3", tier, len(sources)),
			})
		}
	}
	return findings
}

func lintEndingCoverage(s *Scenario) []LintFinding {
	var success, failure int
	for _, ending := range s.Endings {
		switch ending.Kind {
		case "success":
			success++
		case "failure":
			failure++
		}
	}
	var findings []LintFinding
	if success == 0 {
		findings = append(findings, LintFinding{
			Severity: LintWarning,
			Code:     "ending_no_success",
			Message:  "scenario has no success ending",
		})
	}
	if failure == 0 {
		findings = append(findings, LintFinding{
			Severity: LintInfo,
			Code:     "ending_no_failure",
			Message:  "scenario has no failure ending",
		})
	}
	return findings
}

func FormatLintReport(report LintReport) string {
	lines := []string{
		fmt.Sprintf("%s Playability Report", firstNonEmpty(report.Title, report.ScenarioID, "Scenario")),
		"",
		fmt.Sprintf("Scenario: %s", report.ScenarioID),
		fmt.Sprintf("Content: %d locations · %d NPCs · %d clues · %d items · %d triggers · %d endings",
			report.Locations, report.NPCs, report.Clues, report.Items, report.Triggers, report.Endings),
	}
	if len(report.Stages) > 0 {
		lines = append(lines, "", "Stages")
		for _, stage := range report.Stages {
			title := stage.Title
			if title != "" {
				title = " — " + title
			}
			lines = append(lines, fmt.Sprintf("- %s%s", stage.Stage, title))
			lines = append(lines, fmt.Sprintf("  actions: %d leads · %d dialogue · %d item",
				stage.LocationLeads, stage.DialogueOptions, stage.ItemActions))
			if len(stage.ReachableClues) > 0 {
				lines = append(lines, "  reachable clues: "+strings.Join(stage.ReachableClues, ", "))
			}
		}
	}
	if len(report.Findings) > 0 {
		lines = append(lines, "", "Findings")
		for _, finding := range report.Findings {
			lines = append(lines, formatFinding(finding))
		}
	} else {
		lines = append(lines, "", "Findings", "- ok: no base scenario findings")
	}
	if len(report.Variants) > 0 {
		lines = append(lines, "", "Variants")
		for _, variant := range report.Variants {
			lines = append(lines, "- "+variant.ID)
			if len(variant.Findings) == 0 {
				lines = append(lines, "  ok")
				continue
			}
			for _, finding := range variant.Findings {
				lines = append(lines, "  "+formatFinding(finding))
			}
		}
	}
	return strings.Join(lines, "\n")
}

func FormatLintReports(reports []LintReport) string {
	if len(reports) == 0 {
		return "No scenarios found."
	}
	lines := []string{
		"Bundled Scenario Playability Report",
		"",
		fmt.Sprintf("Scenarios: %d", len(reports)),
	}
	for _, report := range reports {
		status := "ok"
		if report.HasErrors() {
			status = "error"
		} else if lintWarningCount(report) > 0 {
			status = "warning"
		}
		lines = append(lines, "", fmt.Sprintf("## %s (%s)", firstNonEmpty(report.Title, report.ScenarioID), report.ScenarioID))
		lines = append(lines, fmt.Sprintf("status: %s", status))
		lines = append(lines, fmt.Sprintf("content: %d locations · %d NPCs · %d clues · %d variants",
			report.Locations, report.NPCs, report.Clues, len(report.Variants)))
		if len(report.Findings) == 0 {
			lines = append(lines, "findings: ok")
		} else {
			lines = append(lines, "findings:")
			for _, finding := range report.Findings {
				lines = append(lines, "  "+formatFinding(finding))
			}
		}
		if len(report.Variants) > 0 {
			lines = append(lines, "variants:")
			for _, variant := range report.Variants {
				if len(variant.Findings) == 0 {
					lines = append(lines, "  - "+variant.ID+": ok")
					continue
				}
				lines = append(lines, "  - "+variant.ID+":")
				for _, finding := range variant.Findings {
					lines = append(lines, "    "+formatFinding(finding))
				}
			}
		}
	}
	return strings.Join(lines, "\n")
}

func lintWarningCount(report LintReport) int {
	count := 0
	for _, finding := range report.Findings {
		if finding.Severity == LintWarning {
			count++
		}
	}
	for _, variant := range report.Variants {
		for _, finding := range variant.Findings {
			if finding.Severity == LintWarning {
				count++
			}
		}
	}
	return count
}

func formatFinding(f LintFinding) string {
	subject := ""
	if f.Subject != "" {
		subject = " " + f.Subject
	}
	return fmt.Sprintf("- [%s] %s%s: %s", f.Severity, f.Code, subject, f.Message)
}

func nonEmptyStringCount(values []string) int {
	count := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}

func sortedUnique(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sortFindings(findings []LintFinding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return severityRank(findings[i].Severity) < severityRank(findings[j].Severity)
		}
		if findings[i].Code != findings[j].Code {
			return findings[i].Code < findings[j].Code
		}
		return findings[i].Subject < findings[j].Subject
	})
}

func severityRank(severity LintSeverity) int {
	switch severity {
	case LintError:
		return 0
	case LintWarning:
		return 1
	default:
		return 2
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
