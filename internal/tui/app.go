package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

// Runner 是 bubbletea Model 与底层 Orchestrator 的桥接抽象，主要让测试用 fake 替换。
type Runner interface {
	RunTurn(ctx context.Context, userInput string) (orchestrator.TurnResult, error)
	SaveID() string
}

// Binder 是 Runner 的可选扩展：实现该接口表示支持调查员绑定（F6.4）。
// /bind 命令仅当 Runner 实现 Binder 时启用。
type Binder interface {
	BindNewInvestigator(ctx context.Context, inv store.Investigator) error
}

// EntryKind 区分日志条目展示样式。
type EntryKind int

const (
	EntryGM EntryKind = iota
	EntryPlayer
	EntrySystem
	EntryError
	EntryEnding
)

type logEntry struct {
	kind EntryKind
	text string
}

// turnDoneMsg 是 RunTurn 完成后的异步消息。
type turnDoneMsg struct {
	res orchestrator.TurnResult
	err error
}

// snapshotMsg 是初始状态加载（save / investigator / location）完成的消息。
type snapshotMsg struct {
	save     store.Save
	inv      store.Investigator
	location store.Location
	err      error
}

// Model 是 bubbletea 应用状态。
type Model struct {
	runner Runner
	store  *store.Store

	save     store.Save
	inv      store.Investigator
	location store.Location

	log         []logEntry
	input       textinput.Model
	spinner     spinner.Model
	busy        bool
	endingShown bool
	drift       string
	width       int
	height      int
	quitting    bool

	// ctx 用于 RunTurn；通常是 context.Background()，由 main.go 注入。
	ctx context.Context
}

// New 构造一个 Model。
func New(ctx context.Context, runner Runner, st *store.Store, openingNarrative string) Model {
	in := textinput.New()
	in.Placeholder = "你的行动…（/help 查看命令）"
	in.Focus()
	in.CharLimit = 1000

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	m := Model{
		runner: runner,
		store:  st,
		input:  in,
		spinner: sp,
		ctx:    ctx,
	}
	if openingNarrative != "" {
		m.log = append(m.log, logEntry{kind: EntryGM, text: openingNarrative})
	}
	return m
}

// Init 触发初始 snapshot 加载与 spinner tick。
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadSnapshotCmd(), m.spinner.Tick)
}

func (m Model) loadSnapshotCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := m.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		repo := m.store.Repo()
		sv, err := repo.GetSave(ctx, m.runner.SaveID())
		if err != nil {
			return snapshotMsg{err: err}
		}
		msg := snapshotMsg{save: sv}
		if inv, err := repo.GetActiveInvestigator(ctx, sv.ID); err == nil {
			msg.inv = inv
		}
		if sv.CurrentLocationID != "" {
			if loc, err := repo.GetLocation(ctx, sv.CurrentLocationID); err == nil {
				msg.location = loc
			}
		}
		return msg
	}
}

// runTurnCmd 在 goroutine 中调 RunTurn 并把结果包成 turnDoneMsg。
func (m Model) runTurnCmd(input string) tea.Cmd {
	return func() tea.Msg {
		ctx := m.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		res, err := m.runner.RunTurn(ctx, input)
		return turnDoneMsg{res: res, err: err}
	}
}

// Update 处理键盘 / 异步消息。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.busy {
			// 等待中只接受 Ctrl+C
			if msg.Type == tea.KeyCtrlC {
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil
		}
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quitting = true
			return m, tea.Quit
		case tea.KeyEnter:
			text := strings.TrimSpace(m.input.Value())
			m.input.SetValue("")
			if text == "" {
				return m, nil
			}
			return m.handleSubmit(text)
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case snapshotMsg:
		if msg.err != nil {
			m.log = append(m.log, logEntry{kind: EntryError, text: "加载存档失败: " + msg.err.Error()})
			return m, nil
		}
		m.save = msg.save
		m.inv = msg.inv
		m.location = msg.location
		return m, nil

	case turnDoneMsg:
		m.busy = false
		if msg.err != nil {
			m.log = append(m.log, logEntry{kind: EntryError, text: "回合失败: " + msg.err.Error()})
			return m, nil
		}
		m.appendTurnResult(msg.res)
		// 拉一次最新 snapshot 让状态条同步。
		return m, m.loadSnapshotCmd()
	}
	return m, nil
}

// handleSubmit 处理用户提交。返回更新后的 Model 与 Cmd。
func (m Model) handleSubmit(text string) (Model, tea.Cmd) {
	if cmd, ok := parseSlash(text); ok {
		return m.handleCommand(cmd)
	}
	m.log = append(m.log, logEntry{kind: EntryPlayer, text: text})
	m.busy = true
	return m, tea.Batch(m.runTurnCmd(text), m.spinner.Tick)
}

func (m Model) handleCommand(c command) (Model, tea.Cmd) {
	switch c.name {
	case "quit", "exit", "q":
		m.quitting = true
		return m, tea.Quit
	case "help":
		m.log = append(m.log, logEntry{kind: EntrySystem, text: helpText()})
	case "sheet":
		m.log = append(m.log, logEntry{kind: EntrySystem, text: m.renderSheet()})
	case "inventory", "inv":
		m.log = append(m.log, logEntry{kind: EntrySystem, text: m.renderInventory()})
	case "time":
		m.log = append(m.log, logEntry{kind: EntrySystem, text: "当前时段: " + string(m.save.TimeOfDay)})
	case "bind":
		return m.handleBind(c)
	case "hint":
		// /hint 走 RunTurn，把 user-input 改为系统提示语，让 GM 用环境/NPC 暗示拉回主线，
		// 不直接给答案。GM prompt（gm_system.tmpl §Granularity）已要求"高价值发现先问"，
		// 这里通过显式输入触发暗示。
		text := "[hint] 玩家请求一个不剧透的小提示：请用环境细节或在场 NPC 的轻微暗示，引导玩家关注主线。不要直接给出答案。"
		m.log = append(m.log, logEntry{kind: EntrySystem, text: "（已请求 GM 提示）"})
		m.busy = true
		return m, tea.Batch(m.runTurnCmd(text), m.spinner.Tick)
	case "talk":
		// /talk <NPC>：把后续输入当作"对该 NPC 说话"，MVP 实现为 prefix 注入并立即提交。
		if c.arg == "" {
			m.log = append(m.log, logEntry{kind: EntryError, text: "用法: /talk <NPC name|id>"})
			return m, nil
		}
		text := fmt.Sprintf("[talk:%s] %s", c.arg, c.rest)
		m.log = append(m.log, logEntry{kind: EntryPlayer, text: text})
		m.busy = true
		return m, tea.Batch(m.runTurnCmd(text), m.spinner.Tick)
	case "all":
		text := "[all] " + c.rest
		m.log = append(m.log, logEntry{kind: EntryPlayer, text: text})
		m.busy = true
		return m, tea.Batch(m.runTurnCmd(text), m.spinner.Tick)
	default:
		m.log = append(m.log, logEntry{kind: EntryError, text: "未知命令: /" + c.name + "（/help 查看可用命令）"})
	}
	return m, nil
}

func (m *Model) appendTurnResult(res orchestrator.TurnResult) {
	if res.Narrative != "" {
		m.log = append(m.log, logEntry{kind: EntryGM, text: res.Narrative})
	}
	for _, fired := range res.Fired {
		m.log = append(m.log, logEntry{
			kind: EntrySystem,
			text: fmt.Sprintf("[剧本] 触发器 %s 已生效", fired.ID),
		})
	}
	switch res.Drift.String() {
	case "soft":
		m.drift = "软偏离：建议关注主线"
	case "hard":
		m.drift = "硬偏离：剧本进入失败收束"
	default:
		m.drift = ""
	}
	if !res.SLAReport.Passed && len(res.SLAReport.Violations) > 0 {
		m.log = append(m.log, logEntry{
			kind: EntryError,
			text: fmt.Sprintf("[SLA 降级] 仍有 %d 项未通过——叙事可能与状态不一致", len(res.SLAReport.Violations)),
		})
	}
	if res.Ending != nil && !m.endingShown {
		m.endingShown = true
		m.log = append(m.log, logEntry{
			kind: EntryEnding,
			text: fmt.Sprintf("[%s] %s", strings.ToUpper(res.Ending.Kind), res.Ending.Description),
		})
	}
}

// View 渲染整个界面。
func (m Model) View() string {
	if m.quitting {
		return "已退出。\n"
	}

	title := titleBar.Render(fmt.Sprintf("Whisperer · %s · 第 %d 回合 · %s",
		m.save.ScenarioID, m.save.TurnCount, m.save.TimeOfDay))

	var body strings.Builder
	for _, e := range m.log {
		body.WriteString(renderEntry(e))
		body.WriteString("\n")
	}

	statusLeft := fmt.Sprintf("HP %d · MP %d · SAN %d", m.inv.HP, m.inv.MP, m.inv.SAN)
	if m.inv.Name != "" {
		statusLeft = m.inv.Name + " · " + statusLeft
	}
	statusRight := "@ " + m.location.Name
	if m.location.Name == "" {
		statusRight = "@ ?"
	}
	if m.drift != "" {
		statusRight += "  ⚠ " + m.drift
	}
	status := statusBar.Render(statusLeft + "    " + statusRight)

	prompt := "> " + m.input.View()
	if m.busy {
		prompt = m.spinner.View() + " 等待 GM…"
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		body.String(),
		status,
		prompt,
	)
}

func renderEntry(e logEntry) string {
	switch e.kind {
	case EntryGM:
		return gmStyle.Render("[GM] ") + e.text
	case EntryPlayer:
		return playerStyle.Render("[你] ") + e.text
	case EntrySystem:
		return systemStyle.Render(e.text)
	case EntryError:
		return errorStyle.Render(e.text)
	case EntryEnding:
		return endingStyle.Render(e.text)
	}
	return e.text
}

func helpText() string {
	return strings.Join([]string{
		"可用命令:",
		"  /sheet                 显示调查员属性",
		"  /inventory (/inv)      显示背包",
		"  /time                  显示当前时段",
		"  /talk <NPC> <话>       指名对某 NPC 说话",
		"  /all <话>              对全场说话",
		"  /hint                  请求一个不剧透的提示",
		"  /bind <name> <职业>    在结局后绑定新调查员到本剧本（F6.4）",
		"  /help                  显示本帮助",
		"  /quit (/exit /q)       退出",
	}, "\n")
}

// handleBind 实现 /bind <name> <occupation>。
//
// 仅在 ending 已展示时启用，避免玩家在游戏中途意外替换调查员。属性走默认值，
// 真正的"分步建卡流程"留给后续 milestone（需求文档 F1.4）。
func (m Model) handleBind(c command) (Model, tea.Cmd) {
	if !m.endingShown {
		m.log = append(m.log, logEntry{kind: EntryError, text: "/bind 仅在剧本结束后可用"})
		return m, nil
	}
	binder, ok := m.runner.(Binder)
	if !ok {
		m.log = append(m.log, logEntry{kind: EntryError, text: "/bind 在当前 runner 上不可用"})
		return m, nil
	}
	if c.arg == "" {
		m.log = append(m.log, logEntry{kind: EntryError, text: "用法: /bind <名字> <职业>"})
		return m, nil
	}
	parts := strings.SplitN(c.rest, " ", 2)
	name := parts[0]
	occupation := "调查员"
	if len(parts) == 2 {
		occupation = strings.TrimSpace(parts[1])
	}
	inv := store.Investigator{
		ID:            "inv-" + name,
		Name:          name,
		Occupation:    occupation,
		AttrsJSON:     `{"STR":50,"CON":60,"SIZ":55,"DEX":60,"APP":50,"INT":75,"POW":60,"EDU":80}`,
		SkillsJSON:    `{"Spot Hidden":50,"Library Use":60,"Listen":40,"Psychology":40}`,
		HP:            12, MP: 12, SAN: 60,
		InventoryJSON: `[]`,
	}
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := binder.BindNewInvestigator(ctx, inv); err != nil {
		m.log = append(m.log, logEntry{kind: EntryError, text: "绑定失败: " + err.Error()})
		return m, nil
	}
	m.log = append(m.log, logEntry{
		kind: EntrySystem,
		text: fmt.Sprintf("已绑定新调查员: %s（%s）。剧本进度保留。", name, occupation),
	})
	m.endingShown = false
	return m, m.loadSnapshotCmd()
}

func (m Model) renderSheet() string {
	if m.inv.Name == "" {
		return "（尚未加载调查员）"
	}
	return fmt.Sprintf(
		"调查员: %s（%s）\nHP %d · MP %d · SAN %d\n属性: %s\n技能: %s",
		m.inv.Name, m.inv.Occupation, m.inv.HP, m.inv.MP, m.inv.SAN,
		m.inv.AttrsJSON, m.inv.SkillsJSON,
	)
}

func (m Model) renderInventory() string {
	if m.inv.InventoryJSON == "" {
		return "（背包为空）"
	}
	return "背包: " + m.inv.InventoryJSON
}
