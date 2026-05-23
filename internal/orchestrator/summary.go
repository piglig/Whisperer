package orchestrator

import (
	"context"
	"fmt"
	"sort"

	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

type TurnSummary struct {
	LocationChange *ValueChange    `json:"location_change,omitempty"`
	TimeChange     *ValueChange    `json:"time_change,omitempty"`
	StageChange    *ValueChange    `json:"stage_change,omitempty"`
	NewClues       []SummaryClue   `json:"new_clues,omitempty"`
	NPCChanges     []SummaryNPC    `json:"npc_changes,omitempty"`
	ThreatChanges  []SummaryThreat `json:"threat_changes,omitempty"`
	FiredTriggers  []string        `json:"fired_triggers,omitempty"`
}

type ValueChange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type SummaryClue struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type SummaryNPC struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	AliveChanged bool   `json:"alive_changed,omitempty"`
	AliveFrom    bool   `json:"alive_from,omitempty"`
	AliveTo      bool   `json:"alive_to,omitempty"`
	RelationFrom int    `json:"relation_from,omitempty"`
	RelationTo   int    `json:"relation_to,omitempty"`
}

type SummaryThreat struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	From     string `json:"from"`
	To       string `json:"to"`
	Severity int    `json:"severity"`
}

func (s TurnSummary) Empty() bool {
	return s.LocationChange == nil &&
		s.TimeChange == nil &&
		s.StageChange == nil &&
		len(s.NewClues) == 0 &&
		len(s.NPCChanges) == 0 &&
		len(s.ThreatChanges) == 0 &&
		len(s.FiredTriggers) == 0
}

type turnSnapshot struct {
	Save     store.Save
	Location string
	Clues    map[string]store.Clue
	NPCs     map[string]store.NPC
	Threats  map[string]scenario.ThreatStatus
}

func captureTurnSnapshot(ctx context.Context, repo *store.Repository, saveID string, scn *scenario.Scenario) (turnSnapshot, error) {
	sv, err := repo.GetSave(ctx, saveID)
	if err != nil {
		return turnSnapshot{}, err
	}
	snap := turnSnapshot{
		Save:    sv,
		Clues:   map[string]store.Clue{},
		NPCs:    map[string]store.NPC{},
		Threats: map[string]scenario.ThreatStatus{},
	}
	if sv.CurrentLocationID != "" {
		if loc, err := repo.GetLocation(ctx, sv.CurrentLocationID); err == nil {
			snap.Location = loc.Name
		}
	}
	clues, err := repo.ListFoundClues(ctx, saveID)
	if err != nil {
		return turnSnapshot{}, fmt.Errorf("list found clues: %w", err)
	}
	for _, clue := range clues {
		snap.Clues[clue.ID] = clue
	}
	if scn != nil {
		for _, snpc := range scn.NPCs {
			npc, err := repo.GetNPC(ctx, snpc.ID)
			if err != nil {
				continue
			}
			snap.NPCs[npc.ID] = npc
		}
		threats, err := scenario.New(scn, repo, nil).Threats(ctx, saveID)
		if err != nil {
			return turnSnapshot{}, fmt.Errorf("threats: %w", err)
		}
		for _, threat := range threats {
			snap.Threats[threat.ID] = threat
		}
	}
	return snap, nil
}

func buildTurnSummary(before, after turnSnapshot, fired []scenario.FiredTrigger) TurnSummary {
	summary := TurnSummary{}
	if before.Save.CurrentLocationID != after.Save.CurrentLocationID {
		summary.LocationChange = &ValueChange{From: firstNonEmpty(before.Location, before.Save.CurrentLocationID), To: firstNonEmpty(after.Location, after.Save.CurrentLocationID)}
	}
	if before.Save.TimeOfDay != after.Save.TimeOfDay {
		summary.TimeChange = &ValueChange{From: string(before.Save.TimeOfDay), To: string(after.Save.TimeOfDay)}
	}
	if before.Save.Stage != after.Save.Stage {
		summary.StageChange = &ValueChange{From: before.Save.Stage, To: after.Save.Stage}
	}
	for id, clue := range after.Clues {
		if _, ok := before.Clues[id]; ok {
			continue
		}
		summary.NewClues = append(summary.NewClues, SummaryClue{ID: id, Description: clue.Description})
	}
	sort.Slice(summary.NewClues, func(i, j int) bool { return summary.NewClues[i].ID < summary.NewClues[j].ID })
	for id, afterNPC := range after.NPCs {
		beforeNPC, ok := before.NPCs[id]
		if !ok {
			continue
		}
		change := SummaryNPC{ID: id, Name: firstNonEmpty(afterNPC.Name, id)}
		changed := false
		if beforeNPC.Alive != afterNPC.Alive {
			change.AliveChanged = true
			change.AliveFrom = beforeNPC.Alive
			change.AliveTo = afterNPC.Alive
			changed = true
		}
		if beforeNPC.RelationToPlayer != afterNPC.RelationToPlayer {
			change.RelationFrom = beforeNPC.RelationToPlayer
			change.RelationTo = afterNPC.RelationToPlayer
			changed = true
		}
		if changed {
			summary.NPCChanges = append(summary.NPCChanges, change)
		}
	}
	sort.Slice(summary.NPCChanges, func(i, j int) bool { return summary.NPCChanges[i].ID < summary.NPCChanges[j].ID })
	for id, afterThreat := range after.Threats {
		beforeThreat, ok := before.Threats[id]
		if !ok || beforeThreat.StateID == afterThreat.StateID {
			continue
		}
		summary.ThreatChanges = append(summary.ThreatChanges, SummaryThreat{
			ID:       id,
			Name:     afterThreat.Name,
			From:     beforeThreat.StateLabel,
			To:       afterThreat.StateLabel,
			Severity: afterThreat.Severity,
		})
	}
	sort.Slice(summary.ThreatChanges, func(i, j int) bool { return summary.ThreatChanges[i].ID < summary.ThreatChanges[j].ID })
	for _, trigger := range fired {
		summary.FiredTriggers = append(summary.FiredTriggers, trigger.ID)
	}
	sort.Strings(summary.FiredTriggers)
	return summary
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
