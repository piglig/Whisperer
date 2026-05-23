package fogharbor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/zhuzhenwu/whisperer/internal/investigator"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

type PlaytestPath string

const (
	PathMainline  PlaytestPath = "mainline"
	PathFlee      PlaytestPath = "flee"
	PathDismissed PlaytestPath = "dismissed"
)

type PlaytestReport struct {
	ScenarioID      string   `json:"scenario_id"`
	VariantID       string   `json:"variant_id,omitempty"`
	Path            string   `json:"path"`
	Turns           int      `json:"turns"`
	EndingID        string   `json:"ending_id,omitempty"`
	EndingKind      string   `json:"ending_kind,omitempty"`
	Expected        []string `json:"expected"`
	Passed          bool     `json:"passed"`
	FoundKeyClues   []string `json:"found_key_clues,omitempty"`
	MissingKeyClues []string `json:"missing_key_clues,omitempty"`
	FiredTriggers   []string `json:"fired_triggers,omitempty"`
	StagePath       []string `json:"stage_path,omitempty"`
	Notes           []string `json:"notes,omitempty"`
}

type PlaytestOptions struct {
	VariantID string
	Path      PlaytestPath
}

func RunPlaytest(ctx context.Context, opts PlaytestOptions) (PlaytestReport, error) {
	if opts.Path == "" {
		opts.Path = PathMainline
	}
	base, err := scenario.LoadBundled("fog_harbor")
	if err != nil {
		return PlaytestReport{}, err
	}
	scn, variantID, err := selectVariant(base, opts.VariantID)
	if err != nil {
		return PlaytestReport{}, err
	}
	env, err := newPlaytestEnv(ctx, scn, variantID)
	if err != nil {
		return PlaytestReport{}, err
	}
	defer env.store.Close()

	switch opts.Path {
	case PathMainline:
		err = runMainline(ctx, env)
	case PathFlee:
		err = runFlee(ctx, env)
	case PathDismissed:
		err = runDismissed(ctx, env)
	default:
		err = fmt.Errorf("unknown fog harbor playtest path %q", opts.Path)
	}
	if err != nil {
		return PlaytestReport{}, err
	}
	return env.report(ctx, opts.Path)
}

func RunAllPlaytests(ctx context.Context) ([]PlaytestReport, error) {
	base, err := scenario.LoadBundled("fog_harbor")
	if err != nil {
		return nil, err
	}
	var variants []string
	for _, variant := range base.Variants {
		variants = append(variants, variant.ID)
	}
	if len(variants) == 0 {
		variants = []string{""}
	}
	var reports []PlaytestReport
	for _, variantID := range variants {
		report, err := RunPlaytest(ctx, PlaytestOptions{VariantID: variantID, Path: PathMainline})
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	for _, path := range []PlaytestPath{PathFlee, PathDismissed} {
		report, err := RunPlaytest(ctx, PlaytestOptions{VariantID: variants[0], Path: path})
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, nil
}

func HasFailures(reports []PlaytestReport) bool {
	for _, report := range reports {
		if !report.Passed {
			return true
		}
	}
	return false
}

func FormatPlaytestReport(report PlaytestReport) string {
	lines := []string{
		"Fog Harbor Playtest Report",
		"",
		"variant: " + nonEmpty(report.VariantID, "base"),
		"path: " + report.Path,
		fmt.Sprintf("turns: %d", report.Turns),
		"ending: " + nonEmpty(report.EndingID, "<none>"),
		"expected: " + strings.Join(report.Expected, ", "),
		fmt.Sprintf("result: %s", passLabel(report.Passed)),
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

func FormatPlaytestReports(reports []PlaytestReport) string {
	if len(reports) == 1 {
		return FormatPlaytestReport(reports[0])
	}
	lines := []string{
		"Fog Harbor Playtest Report",
		"",
		fmt.Sprintf("runs: %d", len(reports)),
	}
	for _, report := range reports {
		lines = append(lines, fmt.Sprintf("- %s / %s: %s (%s, %d turns)",
			nonEmpty(report.VariantID, "base"), report.Path, passLabel(report.Passed), nonEmpty(report.EndingID, "<none>"), report.Turns))
	}
	return strings.Join(lines, "\n")
}

func MarshalJSONReports(reports []PlaytestReport, forceArray bool) ([]byte, error) {
	if len(reports) == 1 && !forceArray {
		return json.MarshalIndent(reports[0], "", "  ")
	}
	return json.MarshalIndent(reports, "", "  ")
}

type playtestEnv struct {
	store     *store.Store
	repo      *store.Repository
	scn       *scenario.Scenario
	engine    *scenario.Engine
	saveID    string
	variantID string
	stagePath []string
}

func newPlaytestEnv(ctx context.Context, scn *scenario.Scenario, variantID string) (*playtestEnv, error) {
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		return nil, err
	}
	repo := st.Repo()
	saveID := uuid.NewString()
	if err := repo.CreateSave(ctx, store.Save{ID: saveID, Name: "fog-harbor-playtest", ScenarioID: scn.ID, VariantID: variantID}); err != nil {
		st.Close()
		return nil, err
	}
	if err := repo.UpsertInvestigator(ctx, investigator.NewFromTemplate(saveID, investigator.Default())); err != nil {
		st.Close()
		return nil, err
	}
	engine := scenario.New(scn, repo, nil)
	if err := engine.Apply(ctx, saveID); err != nil {
		st.Close()
		return nil, err
	}
	env := &playtestEnv{store: st, repo: repo, scn: scn, engine: engine, saveID: saveID, variantID: variantID}
	if err := env.recordStage(ctx); err != nil {
		st.Close()
		return nil, err
	}
	return env, nil
}

func selectVariant(base *scenario.Scenario, id string) (*scenario.Scenario, string, error) {
	if id != "" {
		return scenario.SelectVariantByID(base, id)
	}
	if len(base.Variants) > 0 {
		return scenario.SelectVariantByID(base, base.Variants[0].ID)
	}
	return scenario.SelectVariantByID(base, "")
}

func runMainline(ctx context.Context, env *playtestEnv) error {
	steps := []func(context.Context, *playtestEnv) error{
		func(ctx context.Context, e *playtestEnv) error {
			return e.find(ctx, 1, "harbor", "blood_letter", "tide_chart")
		},
		func(ctx context.Context, e *playtestEnv) error { return e.relateAt(ctx, 2, "station", "helena", 20) },
		func(ctx context.Context, e *playtestEnv) error {
			return e.relateAt(ctx, 4, "church", "father_calvin", 20)
		},
		func(ctx context.Context, e *playtestEnv) error { return e.visit(ctx, 8, "lighthouse", store.TimeNight) },
		func(ctx context.Context, e *playtestEnv) error { return e.visit(ctx, 10, "pub", store.TimeNight) },
		func(ctx context.Context, e *playtestEnv) error {
			return e.visit(ctx, 14, "lighthouse", store.TimeNight)
		},
		func(ctx context.Context, e *playtestEnv) error {
			return e.find(ctx, 16, "reef_cave", "reef_carvings", "sacrifice_chamber")
		},
		func(ctx context.Context, e *playtestEnv) error { return e.confrontCulprit(ctx, 21) },
	}
	for _, step := range steps {
		if err := step(ctx, env); err != nil {
			return err
		}
	}
	return nil
}

func runFlee(ctx context.Context, env *playtestEnv) error {
	steps := []func(context.Context, *playtestEnv) error{
		func(ctx context.Context, e *playtestEnv) error {
			return e.find(ctx, 1, "harbor", "blood_letter", "tide_chart")
		},
		func(ctx context.Context, e *playtestEnv) error {
			return e.relateAt(ctx, 4, "church", "father_calvin", 20)
		},
		func(ctx context.Context, e *playtestEnv) error { return e.visit(ctx, 7, "pub", store.TimeNight) },
		func(ctx context.Context, e *playtestEnv) error { return e.visit(ctx, 11, "harbor", store.TimeMorning) },
	}
	for _, step := range steps {
		if err := step(ctx, env); err != nil {
			return err
		}
	}
	return nil
}

func runDismissed(ctx context.Context, env *playtestEnv) error {
	for turn, delta := range []int{-5, -5, -5, -5} {
		if err := env.relateAt(ctx, turn+1, "station", "rourke", delta); err != nil {
			return err
		}
	}
	return env.visit(ctx, 8, "station", store.TimeMorning)
}

func (e *playtestEnv) confrontCulprit(ctx context.Context, turn int) error {
	switch e.variantID {
	case "calvin_directs":
		return e.visit(ctx, turn, "church", store.TimeNight)
	case "rourke_runs":
		return e.find(ctx, turn, "station", "rourke_bribe")
	default:
		return e.visit(ctx, turn, "pub", store.TimeNight)
	}
}

func (e *playtestEnv) find(ctx context.Context, turn int, location string, clues ...string) error {
	if err := e.visit(ctx, turn, location, ""); err != nil {
		return err
	}
	for _, clue := range clues {
		if err := e.repo.MarkClueFound(ctx, clue, location, turn); err != nil {
			return fmt.Errorf("mark clue %s: %w", clue, err)
		}
	}
	return e.evaluate(ctx)
}

func (e *playtestEnv) relateAt(ctx context.Context, turn int, location, npc string, delta int) error {
	if err := e.visit(ctx, turn, location, ""); err != nil {
		return err
	}
	if err := e.repo.UpdateNPCRelation(ctx, npc, delta); err != nil {
		return fmt.Errorf("update relation %s: %w", npc, err)
	}
	return e.evaluate(ctx)
}

func (e *playtestEnv) visit(ctx context.Context, turn int, location string, tod store.TimeOfDay) error {
	if tod != "" {
		if err := e.repo.SetTimeOfDay(ctx, e.saveID, tod); err != nil {
			return err
		}
	}
	if err := e.repo.UpdateSaveProgress(ctx, e.saveID, location, turn); err != nil {
		return err
	}
	if err := e.repo.MarkLocationVisited(ctx, location); err != nil {
		return err
	}
	return e.evaluate(ctx)
}

func (e *playtestEnv) evaluate(ctx context.Context) error {
	if _, err := e.engine.Evaluate(ctx, e.saveID); err != nil {
		return err
	}
	return e.recordStage(ctx)
}

func (e *playtestEnv) recordStage(ctx context.Context) error {
	save, err := e.repo.GetSave(ctx, e.saveID)
	if err != nil {
		return err
	}
	if len(e.stagePath) == 0 || e.stagePath[len(e.stagePath)-1] != save.Stage {
		e.stagePath = append(e.stagePath, save.Stage)
	}
	return nil
}

func (e *playtestEnv) report(ctx context.Context, path PlaytestPath) (PlaytestReport, error) {
	save, err := e.repo.GetSave(ctx, e.saveID)
	if err != nil {
		return PlaytestReport{}, err
	}
	end, err := e.engine.CheckEndings(ctx, e.saveID)
	if err != nil {
		return PlaytestReport{}, err
	}
	found, err := e.repo.ListFoundClues(ctx, e.saveID)
	if err != nil {
		return PlaytestReport{}, err
	}
	foundSet := map[string]bool{}
	for _, clue := range found {
		foundSet[clue.ID] = true
	}
	events, err := e.repo.ListEvents(ctx, e.saveID, 0, 0)
	if err != nil {
		return PlaytestReport{}, err
	}
	var fired []string
	for _, event := range events {
		if string(event.Type) == scenario.FiredEventType {
			fired = append(fired, event.Description)
		}
	}
	report := PlaytestReport{
		ScenarioID:      e.scn.ID,
		VariantID:       e.variantID,
		Path:            string(path),
		Turns:           save.TurnCount,
		Expected:        expectedEndings(path),
		FoundKeyClues:   foundKeyClues(e.scn.KeyClues, foundSet),
		MissingKeyClues: missingKeyClues(e.scn.KeyClues, foundSet),
		FiredTriggers:   sortedUnique(fired),
		StagePath:       append([]string(nil), e.stagePath...),
	}
	if end != nil {
		report.EndingID = end.ID
		report.EndingKind = end.Kind
	}
	report.Passed = endingAllowed(report.EndingID, report.Expected)
	report.Notes = playtestNotes(path, report)
	return report, nil
}

func expectedEndings(path PlaytestPath) []string {
	switch path {
	case PathMainline:
		return []string{"pact_broken", "solved"}
	case PathFlee:
		return []string{"flee_with_truth"}
	case PathDismissed:
		return []string{"dismissed"}
	default:
		return nil
	}
}

func playtestNotes(path PlaytestPath, report PlaytestReport) []string {
	var notes []string
	if report.EndingID == "" {
		notes = append(notes, "no ending reached")
	}
	if !contains(report.StagePath, scenario.DefaultStage) {
		notes = append(notes, "opening stage was not recorded")
	}
	if path == PathMainline && !contains(report.StagePath, "confrontation") {
		notes = append(notes, "mainline did not reach confrontation stage")
	}
	if len(report.MissingKeyClues) > 0 && path == PathMainline {
		notes = append(notes, "mainline missing key clues: "+strings.Join(report.MissingKeyClues, ", "))
	}
	return notes
}

func endingAllowed(id string, allowed []string) bool {
	for _, value := range allowed {
		if id == value {
			return true
		}
	}
	return false
}

func foundKeyClues(keyClues []string, found map[string]bool) []string {
	var out []string
	for _, clue := range keyClues {
		if found[clue] {
			out = append(out, clue)
		}
	}
	return out
}

func missingKeyClues(keyClues []string, found map[string]bool) []string {
	var out []string
	for _, clue := range keyClues {
		if !found[clue] {
			out = append(out, clue)
		}
	}
	return out
}

func ValidatePath(path string) (PlaytestPath, error) {
	switch PlaytestPath(path) {
	case PathMainline, PathFlee, PathDismissed:
		return PlaytestPath(path), nil
	default:
		return "", errors.New("path must be one of: mainline, flee, dismissed")
	}
}

func passLabel(pass bool) string {
	if pass {
		return "pass"
	}
	return "fail"
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
