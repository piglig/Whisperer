package scenario

// DialogueOptionsFor returns the player-facing dialogue options currently
// available for an NPC.
func DialogueOptionsFor(npc SNPC, stage string, foundClues map[string]bool) []DialogueOption {
	out := make([]DialogueOption, 0, len(npc.DialogueOptions))
	for _, opt := range npc.DialogueOptions {
		if !dialogueStageAllowed(opt, stage) {
			continue
		}
		if !dialogueCluesSatisfied(opt, foundClues) {
			continue
		}
		out = append(out, opt)
	}
	return out
}

func dialogueStageAllowed(opt DialogueOption, stage string) bool {
	if len(opt.Stages) == 0 {
		return true
	}
	for _, allowed := range opt.Stages {
		if allowed == stage {
			return true
		}
	}
	return false
}

func dialogueCluesSatisfied(opt DialogueOption, foundClues map[string]bool) bool {
	for _, clue := range opt.RequiresClues {
		if !foundClues[clue] {
			return false
		}
	}
	for _, clue := range opt.SuppressIfClues {
		if foundClues[clue] {
			return false
		}
	}
	return true
}
