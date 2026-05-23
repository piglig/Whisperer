package tui

import (
	"fmt"
	"strings"

	"github.com/zhuzhenwu/whisperer/internal/fogharbor"
	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
)

func (m Model) briefLine() string {
	parts := []string{
		"地点 " + nonEmpty(m.location.Name, "?"),
		"阶段 " + displayStage(m.currentStage()),
		fmt.Sprintf("HP %d MP %d SAN %d", m.inv.HP, m.inv.MP, m.inv.SAN),
	}
	if alert := m.latestErrorSummary(); alert != "" {
		parts = append(parts, "注意 "+alert)
	} else if m.drift != "" {
		parts = append(parts, m.drift)
	} else {
		parts = append(parts, "主线稳定")
	}
	return strings.Join(parts, " · ")
}

func (m Model) primarySuggestion() string {
	if m.busy {
		return "等待 GM 回应；可用 Ctrl+C 中断程序"
	}
	if advice := m.directorAdvice(); advice.SuggestedAction != "" {
		return advice.SuggestedAction
	}
	if action := m.selectedActionLabel(); action != "" {
		if kind := m.selectedActionKindLabel(); kind != "" {
			return kind + " · " + action
		}
		return action
	}
	for _, npc := range m.npcs {
		if npc.ID != "" {
			return fmt.Sprintf("与 %s 交谈：/talk %s <内容>", nonEmpty(npc.Name, npc.ID), npc.ID)
		}
	}
	if len(m.clues) == 0 {
		return "调查环境，或输入 /hint 获取不剧透提示"
	}
	return "围绕最新线索继续追问，或用 /sheet 检查调查员状态"
}

func (m Model) currentObjective() scenario.Objective {
	return scenario.ObjectiveForStage(m.scn, m.currentStage())
}

func (m Model) currentStage() string {
	if m.stage != "" {
		if stage := scenario.NormalizeStage(m.scn, m.stage); stage != "" {
			return stage
		}
	}
	if m.save.Stage != "" {
		if stage := scenario.NormalizeStage(m.scn, m.save.Stage); stage != "" {
			return stage
		}
	}
	return scenario.DefaultStage
}

func (m Model) actionOptions() []string {
	choices := m.actionChoices()
	out := make([]string, 0, len(choices))
	for _, choice := range choices {
		out = append(out, choice.Label)
	}
	return out
}

func (m Model) actionChoices() []suggestedAction {
	actions := orchestrator.SuggestActions(orchestrator.SuggestionInput{
		Scenario: m.scn,
		Save:     m.save,
		Stage:    m.stage,
		Location: m.location,
		NPCs:     m.npcs,
		Items:    m.items,
		Clues:    m.clues,
		Limit:    5,
	})
	return fogharbor.RankActions(m.directorAdvice(), actions)
}

func (m Model) foundClueSet() map[string]bool {
	found := make(map[string]bool, len(m.clues))
	for _, clue := range m.clues {
		found[clue.ID] = true
	}
	return found
}

func (m Model) scenarioNPC(id string) (scenario.SNPC, bool) {
	if m.scn == nil {
		return scenario.SNPC{}, false
	}
	for _, npc := range m.scn.NPCs {
		if npc.ID == id {
			return npc, true
		}
	}
	return scenario.SNPC{}, false
}

func (m Model) scenarioItem(id string) (scenario.SItem, bool) {
	if m.scn == nil {
		return scenario.SItem{}, false
	}
	for _, item := range m.scn.Items {
		if item.ID == id {
			return item, true
		}
	}
	return scenario.SItem{}, false
}

func (m Model) scenarioLocation() (scenario.SLocation, bool) {
	if m.scn == nil {
		return scenario.SLocation{}, false
	}
	for _, loc := range m.scn.Locations {
		if loc.ID == m.location.ID {
			return loc, true
		}
	}
	return scenario.SLocation{}, false
}
