package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderMainStage(width, height int) string {
	if height < 8 || width < 96 {
		timelineHeight := max(4, height-5)
		railHeight := max(1, height-timelineHeight)
		return lipgloss.JoinVertical(
			lipgloss.Left,
			m.renderTimeline(width, timelineHeight),
			m.renderCaseRail(width, railHeight),
		)
	}

	railWidth := min(38, max(32, width/3))
	gap := 2
	timelineWidth := width - railWidth - gap
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.renderTimeline(timelineWidth, height),
		strings.Repeat(" ", gap),
		m.renderCaseRail(railWidth, height),
	)
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
		if e.kind == EntryGM || e.kind == EntryPlayer || e.kind == EntryEnding {
			out = append(out, e)
		}
	}
	return out
}

func transcriptLines(e logEntry) []string {
	switch e.kind {
	case EntryPlayer:
		return prefixedBody(playerLabelStyle.Render("YOU"), e.text)
	case EntryEnding:
		return prefixedBody(endingStyle.Render("ENDING"), e.text)
	default:
		return prefixedBody(gmLabelStyle.Render("GM"), e.text)
	}
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
