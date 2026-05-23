package tui

import (
	"fmt"
	"strings"

	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

func (m Model) renderCaseRail(width, height int) string {
	return renderPinnedPane("案件卡 · "+m.activePanel.label(), m.caseRailLines(), width, height)
}

func (m Model) caseRailLines() []string {
	lines := []string{m.caseTabsLine(), ""}
	switch m.activePanel {
	case panelPeople:
		return append(lines, m.peoplePanelLines()...)
	case panelClues:
		return append(lines, m.cluePanelLines()...)
	case panelLog:
		return append(lines, m.rulingPanelLines()...)
	default:
		return append(lines, m.scenePanelLines()...)
	}
}

func (m Model) caseTabsLine() string {
	labels := []struct {
		panel sidePanel
		text  string
	}{
		{panelScene, "场景"},
		{panelPeople, "人物"},
		{panelClues, "线索"},
		{panelLog, "裁定"},
	}
	parts := make([]string, 0, len(labels))
	for _, item := range labels {
		if item.panel == m.activePanel {
			parts = append(parts, tabActiveStyle.Render(item.text))
		} else {
			parts = append(parts, tabStyle.Render(item.text))
		}
	}
	return strings.Join(parts, " ")
}

func (m Model) scenePanelLines() []string {
	lines := []string{
		labelStyle.Render("地点"),
		"  " + nonEmpty(m.location.Name, "?"),
	}
	if m.location.Description != "" {
		lines = append(lines, "  "+m.location.Description)
	}
	lines = append(lines,
		"",
		labelStyle.Render("时间"),
		"  "+displayTimeOfDay(m.save.TimeOfDay),
	)
	if m.drift != "" {
		lines = append(lines, "  "+m.drift)
	} else {
		lines = append(lines, "  主线稳定")
	}
	if alert := m.latestErrorSummary(); alert != "" {
		lines = append(lines, "", errorStyle.Render("注意"), "  "+alert)
	}
	if len(m.threats) > 0 {
		lines = append(lines, "", labelStyle.Render("风险"))
		for _, threat := range m.threats {
			lines = append(lines, fmt.Sprintf("  %s：%s", threat.Name, threat.StateLabel))
		}
	}
	if objective := m.currentObjective(); objective.Title != "" {
		lines = append(lines, "", labelStyle.Render("当前目标"), "  "+objective.Title)
		for i, step := range objective.Steps {
			if i >= 2 {
				break
			}
			lines = append(lines, "  - "+step)
		}
	}
	if choices := m.actionChoices(); len(choices) > 0 {
		lines = append(lines, "", labelStyle.Render("可选行动"))
		for i, action := range choices {
			prefix := fmt.Sprintf("  %d. ", i+1)
			if i == m.actionIndex%len(choices) {
				prefix = accentStyle.Render("› " + fmt.Sprintf("%d. ", i+1))
			}
			lines = append(lines, prefix+actionKindLabel(action.Action.Kind)+" · "+action.Label)
		}
	}

	lines = append(lines, "", labelStyle.Render("在场人物"))
	if len(m.npcs) == 0 {
		lines = append(lines, "  暂无")
	} else {
		for _, npc := range m.npcs {
			name := nonEmpty(npc.Name, npc.ID)
			cmd := ""
			if npc.ID != "" {
				cmd = "  /talk " + npc.ID
			}
			lines = append(lines, "  "+name+cmd)
		}
	}

	if advice := m.directorAdvice(); advice.PrimaryObjective != "" {
		lines = append(lines, "", labelStyle.Render("当前重点"))
		lines = append(lines, "  "+urgencyPrefix(advice.Urgency)+advice.PrimaryObjective)
		if advice.Reason != "" {
			lines = append(lines, "  "+advice.Reason)
		}
		if len(advice.BlockedBy) > 0 {
			lines = append(lines, "  缺口："+strings.Join(advice.BlockedBy, "、"))
		}
	}

	lines = append(lines, "", labelStyle.Render("下一步"))
	lines = append(lines, "  "+m.primarySuggestion())
	return lines
}

func (m Model) peoplePanelLines() []string {
	lines := []string{
		labelStyle.Render("调查员"),
		"  " + nonEmpty(m.inv.Name, "未载入") + " / " + nonEmpty(m.inv.Occupation, "-"),
		fmt.Sprintf("  HP %d  MP %d  SAN %d", m.inv.HP, m.inv.MP, m.inv.SAN),
	}
	if m.inv.InventoryJSON != "" {
		lines = append(lines, "  背包 "+m.inv.InventoryJSON)
	}
	if carried := m.carriedItems(); len(carried) > 0 {
		lines = append(lines, "", labelStyle.Render("携带物"))
		for _, item := range carried {
			lines = append(lines, "  "+item.Name)
			if sitem, ok := m.scenarioItem(item.ID); ok {
				actions := scenario.ItemActionsFor(sitem, item, m.currentStage(), m.location.ID, m.foundClueSet())
				for i, action := range actions {
					if i >= 2 {
						break
					}
					lines = append(lines, "    可用："+action.Label)
				}
			}
		}
	}

	lines = append(lines, "", labelStyle.Render("人物"))
	if len(m.npcs) == 0 {
		return append(lines, "  当前地点没有可交互 NPC")
	}
	for _, npc := range m.npcs {
		name := nonEmpty(npc.Name, npc.ID)
		line := "  " + name
		if npc.ID != "" {
			line += "  /talk " + npc.ID
		}
		lines = append(lines, line)
		if npc.Personality != "" {
			lines = append(lines, "    "+npc.Personality)
		}
		if snpc, ok := m.scenarioNPC(npc.ID); ok {
			options := scenario.DialogueOptionsFor(snpc, m.currentStage(), m.foundClueSet())
			for i, opt := range options {
				if i >= 2 {
					break
				}
				lines = append(lines, "    可问："+opt.Label)
			}
		}
	}
	return lines
}

func (m Model) carriedItems() []store.Item {
	out := []store.Item{}
	for _, item := range m.items {
		if item.OwnerType == store.OwnerInvestigator && !item.Destroyed {
			out = append(out, item)
		}
	}
	return out
}

func (m Model) cluePanelLines() []string {
	lines := []string{
		labelStyle.Render("线索"),
	}
	if len(m.clues) == 0 {
		lines = append(lines, "  尚无线索")
	} else {
		start := max(0, len(m.clues)-6)
		for i := len(m.clues) - 1; i >= start; i-- {
			lines = append(lines, "  "+m.clues[i].Description)
		}
	}

	lines = append(lines, "", labelStyle.Render("最近记录"))
	if len(m.events) == 0 {
		return append(lines, "  暂无事件记录")
	}
	for _, ev := range lastEvents(m.events, 6) {
		lines = append(lines, "  "+ev.Description)
	}
	return lines
}

func (m Model) rulingPanelLines() []string {
	lines := []string{
		labelStyle.Render("裁定记录"),
	}
	rulings := rulingEntries(m.log)
	if len(rulings) == 0 {
		lines = append(lines, "  尚无系统裁定")
	} else {
		for _, entry := range lastLogEntries(rulings, 6) {
			for _, line := range rulingLines(entry) {
				lines = append(lines, "  "+line)
			}
		}
	}

	lines = append(lines,
		"",
		labelStyle.Render("操作"),
		"  Ctrl+P 命令面板",
		"  Shift+Tab 切换案件卡",
		"  PgUp/PgDn 回看故事",
	)
	return lines
}

func rulingEntries(entries []logEntry) []logEntry {
	out := []logEntry{}
	for _, e := range entries {
		if e.kind == EntrySystem || e.kind == EntryError {
			out = append(out, e)
		}
	}
	return out
}

func (m Model) latestErrorSummary() string {
	for i := len(m.log) - 1; i >= 0; i-- {
		if m.log[i].kind == EntryError {
			return summarizeError(m.log[i].text)
		}
	}
	return ""
}

func rulingLines(e logEntry) []string {
	switch e.kind {
	case EntryError:
		return prefixedBody(errorStyle.Render("ERR"), summarizeError(e.text))
	default:
		return prefixedBody(systemStyle.Render("NOTE"), e.text)
	}
}

func lastLogEntries(entries []logEntry, limit int) []logEntry {
	if limit <= 0 || len(entries) == 0 {
		return nil
	}
	if len(entries) <= limit {
		return entries
	}
	return entries[len(entries)-limit:]
}

func lastEvents(events []store.Event, limit int) []store.Event {
	if limit <= 0 || len(events) == 0 {
		return nil
	}
	if len(events) <= limit {
		return events
	}
	return events[len(events)-limit:]
}
