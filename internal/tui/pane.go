package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func renderStoryPane(title string, raw []string, width, height, offset int) string {
	return renderPaneWithScrollOffset(title, raw, width, height, true, offset)
}

func renderPinnedPane(title string, raw []string, width, height int) string {
	return renderPaneWithScrollOffset(title, raw, width, height, false, 0)
}

func renderPaneWithScrollOffset(title string, raw []string, width, height int, tail bool, offset int) string {
	width = max(10, width)
	height = max(1, height)

	lines := []string{paneTitleStyle.Render(fitLine(title, width))}
	if height > 1 {
		lines = append(lines, mutedStyle.Render(strings.Repeat("─", width)))
	}

	bodyHeight := max(0, height-len(lines))
	body := wrapLines(raw, width)
	if bodyHeight > 0 && len(body) > bodyHeight {
		if tail && offset > 0 {
			offset = min(offset, len(body)-bodyHeight)
			start := max(0, len(body)-bodyHeight-offset)
			end := min(len(body), start+bodyHeight)
			body = append([]string{}, body[start:end]...)
			if start > 0 && len(body) > 0 {
				body[0] = mutedStyle.Render("↑ 更早内容")
			}
			if end < len(wrapLines(raw, width)) && len(body) > 0 {
				body[len(body)-1] = mutedStyle.Render("↓ 更新内容")
			}
		} else if tail {
			body = tailPaneLines(body, bodyHeight)
		} else if bodyHeight == 1 {
			body = []string{"..."}
		} else {
			body = append(body[:bodyHeight-1], "...")
		}
	}
	lines = append(lines, body...)
	return fixedLines(lines, width, height)
}

func fixedLines(lines []string, width, height int) string {
	if height <= 0 {
		return ""
	}
	out := append([]string{}, lines...)
	for len(out) < height {
		out = append(out, "")
	}
	if len(out) > height {
		out = out[:height]
	}
	for i, line := range out {
		out[i] = fitLine(line, width)
	}
	return strings.Join(out, "\n")
}

func tailPaneLines(lines []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	if len(lines) <= limit {
		return lines
	}
	if strings.Contains(lines[0], "之前") && limit >= 2 {
		if limit == 2 {
			return []string{lines[0], "..."}
		}
		tail := lines[len(lines)-limit+2:]
		return append([]string{lines[0], "..."}, tail...)
	}
	return tailLines(lines, limit)
}

func wrapLines(lines []string, width int) []string {
	out := []string{}
	for _, line := range lines {
		if line == "" {
			out = append(out, "")
			continue
		}
		out = append(out, wrapLine(line, width)...)
	}
	return out
}

func wrapLine(line string, width int) []string {
	width = max(4, width)
	words := []rune(line)
	if len(words) == 0 {
		return []string{""}
	}
	out := []string{}
	var b strings.Builder
	for _, r := range words {
		next := b.String() + string(r)
		if lipgloss.Width(next) > width {
			out = append(out, b.String())
			b.Reset()
		}
		b.WriteRune(r)
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

func tailLines(lines []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	if len(lines) <= limit {
		return lines
	}
	out := append([]string{"..."}, lines[len(lines)-limit+1:]...)
	return out
}

func fitLine(s string, width int) string {
	width = max(1, width)
	if lipgloss.Width(s) <= width {
		return s + strings.Repeat(" ", width-lipgloss.Width(s))
	}
	var b strings.Builder
	for _, r := range s {
		next := b.String() + string(r)
		if lipgloss.Width(next+"…") > width {
			break
		}
		b.WriteRune(r)
	}
	return b.String() + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
