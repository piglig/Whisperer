package scenario

import "github.com/zhuzhenwu/whisperer/internal/store"

// ItemActionsFor returns currently available player-facing actions for an item.
func ItemActionsFor(item SItem, state store.Item, stage, locationID string, foundClues map[string]bool) []ItemAction {
	if state.Destroyed {
		return nil
	}
	out := make([]ItemAction, 0, len(item.Actions))
	for _, action := range item.Actions {
		if !itemActionStageAllowed(action, stage) {
			continue
		}
		if !itemActionLocationAllowed(action, locationID) {
			continue
		}
		if !itemActionOwnerAllowed(action, state.OwnerType) {
			continue
		}
		if !itemActionCluesSatisfied(action, foundClues) {
			continue
		}
		out = append(out, action)
	}
	return out
}

func itemActionStageAllowed(action ItemAction, stage string) bool {
	if len(action.Stages) == 0 {
		return true
	}
	for _, allowed := range action.Stages {
		if allowed == stage {
			return true
		}
	}
	return false
}

func itemActionLocationAllowed(action ItemAction, locationID string) bool {
	if len(action.Locations) == 0 {
		return true
	}
	for _, allowed := range action.Locations {
		if allowed == locationID {
			return true
		}
	}
	return false
}

func itemActionOwnerAllowed(action ItemAction, ownerType store.OwnerType) bool {
	if len(action.OwnerTypes) == 0 {
		return true
	}
	for _, allowed := range action.OwnerTypes {
		if store.OwnerType(allowed) == ownerType {
			return true
		}
	}
	return false
}

func itemActionCluesSatisfied(action ItemAction, foundClues map[string]bool) bool {
	for _, clue := range action.RequiresClues {
		if !foundClues[clue] {
			return false
		}
	}
	for _, clue := range action.SuppressIfClues {
		if foundClues[clue] {
			return false
		}
	}
	return true
}
