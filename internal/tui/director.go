package tui

import (
	"strings"

	"github.com/zhuzhenwu/whisperer/internal/director"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

func (m Model) directorAdvice() director.Advice {
	return director.EvaluateAdvice(director.Snapshot{
		ScenarioID:     m.save.ScenarioID,
		Stage:          m.currentStage(),
		Turn:           m.save.TurnCount,
		LocationID:     m.location.ID,
		TimeOfDay:      m.save.TimeOfDay,
		FoundClues:     m.foundClueSet(),
		FiredTriggers:  m.fired,
		Threats:        m.threats,
		AvailableNPCs:  m.npcs,
		AvailableItems: m.items,
		Objective:      m.currentObjective(),
	})
}

func urgencyPrefix(urgency director.Urgency) string {
	switch urgency {
	case director.UrgencyCritical:
		return "[高危] "
	case director.UrgencyWarning:
		return "[警告] "
	default:
		return ""
	}
}

func firedTriggerSet(events []store.Event) map[string]bool {
	out := map[string]bool{}
	for _, event := range events {
		if string(event.Type) == scenario.FiredEventType && strings.TrimSpace(event.Description) != "" {
			out[event.Description] = true
		}
	}
	return out
}
