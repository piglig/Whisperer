package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderMainStage(width, height int) string {
	if height < 8 || width < 96 {
		boardHeight := min(8, max(4, height/3))
		timelineHeight := max(3, height-boardHeight-4)
		railHeight := max(1, height-boardHeight-timelineHeight)
		return lipgloss.JoinVertical(
			lipgloss.Left,
			m.renderActionBoard(width, boardHeight),
			m.renderTimeline(width, timelineHeight),
			m.renderCaseRail(width, railHeight),
		)
	}

	railWidth := min(38, max(32, width/3))
	gap := 2
	timelineWidth := width - railWidth - gap
	boardHeight := min(10, max(7, height/3))
	left := lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderActionBoard(timelineWidth, boardHeight),
		m.renderTimeline(timelineWidth, max(3, height-boardHeight)),
	)
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		left,
		strings.Repeat(" ", gap),
		m.renderCaseRail(railWidth, height),
	)
}

func (m Model) renderActionBoard(width, height int) string {
	return renderPinnedPane("案件板 / 可行动项", m.actionBoardLines(), width, height)
}

func (m Model) actionBoardLines() []string {
	lines := []string{}
	objective := m.currentObjective()
	if objective.Title != "" {
		lines = append(lines, accentStyle.Render("目标")+"  "+objective.Title)
		if len(objective.Steps) > 0 {
			lines = append(lines, mutedStyle.Render("推进")+"  "+strings.Join(limitStrings(objective.Steps, 2), " · "))
		}
	} else {
		lines = append(lines, accentStyle.Render("目标")+"  "+m.primarySuggestion())
	}
	lines = append(lines, mutedStyle.Render("位置")+"  "+nonEmpty(m.location.Name, "?")+" · "+displayStage(m.currentStage())+" · "+displayTimeOfDay(m.save.TimeOfDay))

	choices := m.actionChoices()
	if len(choices) == 0 {
		return append(lines, "", "暂无结构化行动；直接描述你的下一步，或输入 /hint。")
	}
	lines = append(lines, "", accentStyle.Render("选择行动")+"  ↑/↓ 选择 · 1-5 填入 · Enter 执行")
	for i, action := range choices {
		prefix := fmt.Sprintf("%d. ", i+1)
		if i == m.actionIndex%len(choices) {
			prefix = accentStyle.Render("› " + prefix)
		} else {
			prefix = "  " + prefix
		}
		lines = append(lines, prefix+actionKindLabel(action.Action.Kind)+" · "+action.Label)
	}
	return lines
}

func (m Model) renderTimeline(width, height int) string {
	return renderStoryPane("故事卷轴 / 行动流", m.timelineLines(), width, height, m.storyOffset)
}

func (m Model) timelineLines() []string {
	entries := timelineEntries(m.log)
	if len(entries) == 0 {
		return []string{
			"GM 准备好了。直接描述你的行动，或输入 /help 查看命令。",
		}
	}

	const visibleEntries = 14
	hidden := 0
	if len(entries) > visibleEntries {
		hidden = len(entries) - visibleEntries
		entries = entries[hidden:]
	}

	lines := []string{}
	if hidden > 0 {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("之前 %d 条行动已折叠", hidden)), "")
	}
	for i, entry := range entries {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, transcriptLines(entry)...)
	}
	return lines
}

func timelineEntries(entries []logEntry) []logEntry {
	out := []logEntry{}
	for _, e := range entries {
		if e.kind == EntryGM || e.kind == EntryPlayer || e.kind == EntryReview || e.kind == EntryEnding {
			out = append(out, e)
		}
	}
	return out
}

func transcriptLines(e logEntry) []string {
	switch e.kind {
	case EntryPlayer:
		return prefixedBody(playerLabelStyle.Render("YOU"), e.text)
	case EntryReview:
		return prefixedBody(systemStyle.Render("RECAP"), e.text)
	case EntryEnding:
		return prefixedBody(endingStyle.Render("ENDING"), e.text)
	default:
		return prefixedBody(gmLabelStyle.Render("GM"), e.text)
	}
}

func limitStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func prefixedBody(label, text string) []string {
	raw := strings.Split(strings.TrimSpace(text), "\n")
	if len(raw) == 0 || raw[0] == "" {
		return []string{label}
	}
	out := []string{label + "  " + raw[0]}
	for _, line := range raw[1:] {
		out = append(out, strings.Repeat(" ", lipgloss.Width(label)+2)+line)
	}
	return out
}
