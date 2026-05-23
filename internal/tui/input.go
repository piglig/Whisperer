package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
)

// handleSubmit 处理用户提交。返回更新后的 Model 与 Cmd。
func (m Model) handleSubmit(text string) (Model, tea.Cmd) {
	if cmd, ok := parseSlash(text); ok {
		return m.handleCommand(cmd)
	}
	m.log = append(m.log, logEntry{kind: EntryPlayer, text: displaySubmittedText(text)})
	m.busy = true
	m.storyOffset = 0
	return m, tea.Batch(m.runTurnCmd(text), m.spinner.Tick)
}

func displaySubmittedText(text string) string {
	if action, ok := orchestrator.DecodePlayerAction(strings.TrimSpace(text)); ok {
		return nonEmpty(action.Text, action.Raw)
	}
	return text
}

func actionDigit(msg tea.KeyMsg) (int, bool) {
	if len(msg.Runes) != 1 {
		return 0, false
	}
	r := msg.Runes[0]
	if r < '1' || r > '9' {
		return 0, false
	}
	return int(r - '1'), true
}

func (m Model) fillAction(idx int) Model {
	actions := m.actionChoices()
	if idx < 0 || idx >= len(actions) {
		return m
	}
	m.actionIndex = idx
	m.input.SetValue(actions[idx].DisplayInput())
	m.input.CursorEnd()
	m.resetCompletion()
	return m
}

func (m Model) moveAction(delta int) Model {
	actions := m.actionChoices()
	if len(actions) == 0 {
		m.actionIndex = 0
		return m
	}
	m.actionIndex = (m.actionIndex + delta + len(actions)) % len(actions)
	return m
}

func (m Model) selectedAction() string {
	actions := m.actionChoices()
	if len(actions) == 0 {
		return ""
	}
	idx := m.actionIndex
	if idx < 0 || idx >= len(actions) {
		idx = 0
	}
	return actions[idx].Input
}

func (m Model) selectedActionLabel() string {
	actions := m.actionChoices()
	if len(actions) == 0 {
		return ""
	}
	idx := m.actionIndex
	if idx < 0 || idx >= len(actions) {
		idx = 0
	}
	return actions[idx].Label
}

func (m Model) selectedActionKindLabel() string {
	actions := m.actionChoices()
	if len(actions) == 0 {
		return ""
	}
	idx := m.actionIndex
	if idx < 0 || idx >= len(actions) {
		idx = 0
	}
	return actionKindLabel(actions[idx].Action.Kind)
}

func actionKindLabel(kind orchestrator.TurnIntent) string {
	switch kind {
	case orchestrator.IntentInvestigate:
		return "调查"
	case orchestrator.IntentTalk:
		return "交谈"
	case orchestrator.IntentMove:
		return "移动"
	case orchestrator.IntentUseItem:
		return "使用"
	case orchestrator.IntentConfront:
		return "对峙"
	case orchestrator.IntentReport:
		return "结案"
	default:
		return "行动"
	}
}

func (m Model) completeInput() Model {
	current := m.input.Value()
	candidates := m.completionCandidates(current)
	if len(candidates) == 0 {
		return m
	}
	if current != m.completionBase {
		m.completionBase = current
		m.completionIndex = 0
	} else {
		m.completionIndex = (m.completionIndex + 1) % len(candidates)
	}
	m.input.SetValue(candidates[m.completionIndex])
	m.input.CursorEnd()
	return m
}

func (m *Model) resetCompletion() {
	m.completionBase = ""
	m.completionIndex = 0
}

func (m Model) completionCandidates(input string) []string {
	text := strings.TrimLeft(input, " ")
	if !strings.HasPrefix(text, "/") {
		return nil
	}
	if strings.HasPrefix(text, "/talk ") {
		return m.talkCompletionCandidates(text)
	}
	return commandCompletionCandidates(text)
}

func commandCompletionCandidates(text string) []string {
	commands := []string{
		"/all ",
		"/bind ",
		"/help",
		"/hint",
		"/inventory",
		"/inv",
		"/quit",
		"/sheet",
		"/talk ",
		"/time",
	}
	out := []string{}
	for _, c := range commands {
		if strings.HasPrefix(c, text) {
			out = append(out, c)
		}
	}
	return out
}

func (m Model) talkCompletionCandidates(text string) []string {
	rest := strings.TrimPrefix(text, "/talk ")
	parts := strings.SplitN(rest, " ", 2)
	prefix := strings.ToLower(parts[0])
	suffix := ""
	if len(parts) == 2 {
		suffix = " " + parts[1]
	}
	out := []string{}
	for _, npc := range m.npcs {
		for _, key := range []string{npc.ID, npc.Name} {
			if key == "" {
				continue
			}
			if prefix == "" || strings.HasPrefix(strings.ToLower(key), prefix) {
				out = append(out, "/talk "+npc.ID+suffix)
				break
			}
		}
	}
	return out
}
