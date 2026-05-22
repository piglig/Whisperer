package orchestrator

import (
	"context"
	"fmt"

	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

func buildCaseReport(ctx context.Context, repo *store.Repository, saveID string, scn *scenario.Scenario, sv store.Save, ending *scenario.Ending, variantID string) (*scenario.CaseReport, error) {
	found, err := repo.ListFoundClues(ctx, saveID)
	if err != nil {
		return nil, fmt.Errorf("list found clues: %w", err)
	}
	npcs := make([]store.NPC, 0, len(scn.NPCs))
	for _, snpc := range scn.NPCs {
		npc, err := repo.GetNPC(ctx, snpc.ID)
		if err != nil {
			continue
		}
		npcs = append(npcs, npc)
	}
	return scenario.BuildCaseReport(scn, scenario.CaseReportInput{
		Save:       sv,
		Ending:     ending,
		VariantID:  variantID,
		FoundClues: found,
		NPCs:       npcs,
	}), nil
}
