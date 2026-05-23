// Package saveselect implements the pre-game save selector TUI screen.
//
// 启动 Whisperer 但未通过 --save 指定时，若存在历史存档就进这一屏；玩家可以选择
// 一个存档进入正式 TUI，新建一个空白存档，或者删除遗留存档。
package saveselect

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zhuzhenwu/whisperer/internal/store"
)

// Action 是选择器的最终输出动作。
type Action int

const (
	// ActionQuit 玩家按 q / Ctrl+C 主动退出，主程序应直接结束。
	ActionQuit Action = iota
	// ActionLoad 玩家选定一个已有存档，Result.SaveID 即目标。
	ActionLoad
	// ActionNew 玩家选择新建存档，主程序走标准 ensureSaveWithVariant 流程。
	ActionNew
)

// Result 是选择器结束后从 Model.Result() 取出的结果。
type Result struct {
	Action Action
	SaveID string
}

// Deleter 是删除存档需要的最小接口，主程序传 store.Store.Repo() 实现即可。
type Deleter interface {
	DeleteSave(ctx context.Context, id string) error
}

// Model 是 bubbletea Model。
type Model struct {
	saves   []store.Save
	deleter Deleter
	// onDelete 在 DeleteSave 成功后调用，用于清理 per-save 资源（memory dir 等）。
	// 返回错误会显示在状态行但不阻塞后续操作。
	onDelete func(saveID string) error

	cursor        int // 0..len(saves) 即 "+ 新建" 行
	pendingDelete int // 等待二次确认的索引；-1 表示无
	status        string
	result        Result
	quitting      bool
	width         int
}

// New 构造一个 Model。saves 应按 updated_at desc 传入；deleter / onDelete 可为 nil。
func New(saves []store.Save, deleter Deleter, onDelete func(string) error) Model {
	return Model{
		saves:         saves,
		deleter:       deleter,
		onDelete:      onDelete,
		pendingDelete: -1,
	}
}

// Result 返回选择器的最终决策。在 prog.Run() 结束后调用。
func (m Model) Result() Result { return m.result }

func (m Model) Init() tea.Cmd { return nil }

// newRowIndex 返回 "+ 新建存档" 行的索引（始终在末尾）。
func (m Model) newRowIndex() int { return len(m.saves) }

// rowCount 返回总行数（存档数 + 新建行）。
func (m Model) rowCount() int { return len(m.saves) + 1 }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// 任何非 d 的键都会取消 pending delete。
	if msg.String() != "d" && m.pendingDelete >= 0 {
		m.pendingDelete = -1
		m.status = ""
	}

	switch msg.String() {
	case "ctrl+c", "q", "esc":
		m.result = Result{Action: ActionQuit}
		m.quitting = true
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case "down", "j":
		if m.cursor < m.rowCount()-1 {
			m.cursor++
		}
		return m, nil

	case "n":
		m.result = Result{Action: ActionNew}
		m.quitting = true
		return m, tea.Quit

	case "enter":
		if m.cursor == m.newRowIndex() {
			m.result = Result{Action: ActionNew}
		} else {
			m.result = Result{Action: ActionLoad, SaveID: m.saves[m.cursor].ID}
		}
		m.quitting = true
		return m, tea.Quit

	case "d":
		if m.cursor == m.newRowIndex() {
			return m, nil
		}
		if m.pendingDelete == m.cursor {
			return m.confirmDelete()
		}
		m.pendingDelete = m.cursor
		m.status = "再按一次 d 确认删除，其他键取消"
		return m, nil
	}
	return m, nil
}

// confirmDelete 执行已经过二次确认的删除。
func (m Model) confirmDelete() (tea.Model, tea.Cmd) {
	target := m.saves[m.pendingDelete]
	m.pendingDelete = -1

	if m.deleter != nil {
		if err := m.deleter.DeleteSave(context.Background(), target.ID); err != nil {
			m.status = fmt.Sprintf("删除失败: %v", err)
			return m, nil
		}
	}
	if m.onDelete != nil {
		if err := m.onDelete(target.ID); err != nil {
			m.status = fmt.Sprintf("已删除存档但清理失败: %v", err)
			// 仍然把它从列表里移除
		} else {
			m.status = fmt.Sprintf("已删除：%s", target.ID)
		}
	} else {
		m.status = fmt.Sprintf("已删除：%s", target.ID)
	}

	m.saves = append(m.saves[:m.cursor], m.saves[m.cursor+1:]...)
	if m.cursor > 0 && m.cursor >= len(m.saves) {
		m.cursor = len(m.saves) // 落到 "+ 新建" 行
	}

	// 删光后引导玩家直接新建
	if len(m.saves) == 0 {
		m.status = "已无存档，按 Enter 新建"
	}
	return m, nil
}

// View 渲染整屏。
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	title := titleStyle.Render("Whisperer · 选择存档")

	var rows []string
	for i, sv := range m.saves {
		rows = append(rows, m.renderSaveRow(i, sv))
	}
	rows = append(rows, mutedStyle.Render("───────────────────"))
	rows = append(rows, m.renderNewRow())

	help := mutedStyle.Render("↑↓/jk 选择   Enter 进入   n 新建   d 删除   q 退出")

	body := lipgloss.JoinVertical(lipgloss.Left, rows...)

	sections := []string{title, "", body, "", help}
	if m.status != "" {
		sections = append(sections, statusStyle.Render(m.status))
	}
	return lipgloss.JoinVertical(lipgloss.Left, sections...) + "\n"
}

func (m Model) renderSaveRow(i int, sv store.Save) string {
	prefix := "  "
	if i == m.cursor {
		prefix = accentStyle.Render("▸ ")
	}
	headline := fmt.Sprintf("%s · %s", saveDisplayName(sv), sv.ScenarioID)
	if sv.VariantID != "" {
		headline += mutedStyle.Render(fmt.Sprintf(" [%s]", sv.VariantID))
	}

	detail := fmt.Sprintf("第 %d 回合 · %s · 更新于 %s",
		sv.TurnCount, displayTimeOfDay(sv.TimeOfDay), formatUpdated(sv.UpdatedAt))
	if m.pendingDelete == i {
		detail = warningStyle.Render("即将删除：再按 d 确认 / 任意键取消")
	}

	line1 := prefix + headline
	if i == m.cursor {
		line1 = highlightStyle.Render(line1)
	}
	line2 := "    " + mutedStyle.Render(detail)
	return lipgloss.JoinVertical(lipgloss.Left, line1, line2)
}

func (m Model) renderNewRow() string {
	prefix := "  "
	label := "+ 新建存档"
	if m.cursor == m.newRowIndex() {
		prefix = accentStyle.Render("▸ ")
		label = highlightStyle.Render(label)
	}
	return prefix + label
}

func saveDisplayName(sv store.Save) string {
	if sv.Name != "" && sv.Name != "untitled" {
		return sv.Name
	}
	if len(sv.ID) >= 8 {
		return sv.ID[:8]
	}
	return sv.ID
}

func displayTimeOfDay(t store.TimeOfDay) string {
	switch t {
	case store.TimeMorning:
		return "morning"
	case store.TimeAfternoon:
		return "afternoon"
	case store.TimeNight:
		return "night"
	}
	return "—"
}

func formatUpdated(ms int64) string {
	if ms == 0 {
		return "—"
	}
	return time.UnixMilli(ms).Format("2006-01-02 15:04")
}

var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#facc15")).
			Bold(true)
	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#64748b"))
	accentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#facc15")).
			Bold(true)
	highlightStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f8fafc")).
			Bold(true)
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#38bdf8"))
	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fca5a5")).
			Bold(true)
)
