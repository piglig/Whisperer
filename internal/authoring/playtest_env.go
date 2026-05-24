package authoring

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/zhuzhenwu/whisperer/internal/investigator"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

type Env struct {
	store     *store.Store
	repo      *store.Repository
	scn       *scenario.Scenario
	engine    *scenario.Engine
	saveID    string
	variantID string
	stagePath []string
}

func NewEnv(ctx context.Context, scn *scenario.Scenario, variantID string) (*Env, error) {
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		return nil, err
	}
	repo := st.Repo()
	saveID := uuid.NewString()
	if err := repo.CreateSave(ctx, store.Save{ID: saveID, Name: scn.ID + "_playtest", ScenarioID: scn.ID, VariantID: variantID}); err != nil {
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
	env := &Env{store: st, repo: repo, scn: scn, engine: engine, saveID: saveID, variantID: variantID}
	if err := env.recordStage(ctx); err != nil {
		st.Close()
		return nil, err
	}
	return env, nil
}

func (e *Env) Close() error {
	if e == nil || e.store == nil {
		return nil
	}
	return e.store.Close()
}

func (e *Env) VariantID() string {
	return e.variantID
}

func (e *Env) Find(ctx context.Context, turn int, location string, clues ...string) error {
	if err := e.Visit(ctx, turn, location, ""); err != nil {
		return err
	}
	for _, clue := range clues {
		if err := e.repo.MarkClueFound(ctx, clue, location, turn); err != nil {
			return fmt.Errorf("mark clue %s: %w", clue, err)
		}
	}
	return e.evaluate(ctx)
}

func (e *Env) RelateAt(ctx context.Context, turn int, location, npc string, delta int) error {
	if err := e.Visit(ctx, turn, location, ""); err != nil {
		return err
	}
	if err := e.repo.UpdateNPCRelation(ctx, npc, delta); err != nil {
		return fmt.Errorf("update relation %s: %w", npc, err)
	}
	return e.evaluate(ctx)
}

func (e *Env) Visit(ctx context.Context, turn int, location string, tod store.TimeOfDay) error {
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

func (e *Env) Report(ctx context.Context, path string, expected []string) (PlaytestReport, error) {
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
	fired := []string{}
	for _, event := range events {
		if string(event.Type) == scenario.FiredEventType {
			fired = append(fired, event.Description)
		}
	}
	report := PlaytestReport{
		ScenarioID:      e.scn.ID,
		VariantID:       e.variantID,
		Path:            path,
		Turns:           save.TurnCount,
		Expected:        expected,
		FoundKeyClues:   foundKeyClues(e.scn.KeyClues, foundSet),
		MissingKeyClues: missingKeyClues(e.scn.KeyClues, foundSet),
		FiredTriggers:   SortedUnique(fired),
		StagePath:       append([]string(nil), e.stagePath...),
	}
	if end != nil {
		report.EndingID = end.ID
		report.EndingKind = end.Kind
	}
	report.Passed = EndingAllowed(report.EndingID, report.Expected)
	report.Notes = PlaytestNotes(path, report)
	return report, nil
}

func (e *Env) evaluate(ctx context.Context) error {
	if _, err := e.engine.Evaluate(ctx, e.saveID); err != nil {
		return err
	}
	return e.recordStage(ctx)
}

func (e *Env) recordStage(ctx context.Context) error {
	save, err := e.repo.GetSave(ctx, e.saveID)
	if err != nil {
		return err
	}
	if len(e.stagePath) == 0 || e.stagePath[len(e.stagePath)-1] != save.Stage {
		e.stagePath = append(e.stagePath, save.Stage)
	}
	return nil
}

func SelectVariant(base *scenario.Scenario, id string) (*scenario.Scenario, string, error) {
	if id != "" {
		return scenario.SelectVariantByID(base, id)
	}
	if len(base.Variants) > 0 {
		return scenario.SelectVariantByID(base, base.Variants[0].ID)
	}
	return scenario.SelectVariantByID(base, "")
}

func VariantIDs(base *scenario.Scenario) []string {
	var variants []string
	if base != nil {
		for _, variant := range base.Variants {
			variants = append(variants, variant.ID)
		}
	}
	if len(variants) == 0 {
		return []string{""}
	}
	return variants
}

func EndingAllowed(id string, allowed []string) bool {
	for _, value := range allowed {
		if id == value {
			return true
		}
	}
	return false
}

func PlaytestNotes(path string, report PlaytestReport) []string {
	var notes []string
	if report.EndingID == "" {
		notes = append(notes, "no ending reached")
	}
	if !Contains(report.StagePath, scenario.DefaultStage) {
		notes = append(notes, "opening stage was not recorded")
	}
	if path == "mainline" && !Contains(report.StagePath, "confrontation") {
		notes = append(notes, "mainline did not reach confrontation stage")
	}
	if len(report.MissingKeyClues) > 0 && path == "mainline" {
		notes = append(notes, "mainline missing key clues: "+strings.Join(report.MissingKeyClues, ", "))
	}
	return notes
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
