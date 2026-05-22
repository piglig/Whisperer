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
	"github.com/zhuzhenwu/whisperer/internal/scenario"
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

type suggestedAction struct {
	label string
	input string
}

// turnDoneMsg 是 RunTurn 完成后的异步消息。
type turnDoneMsg struct {
	res orchestrator.TurnResult
	err error
}

// snapshotMsg 是初始状态加载（save / investigator / location）完成的消息。
type snapshotMsg struct {
	save      store.Save
	inv       store.Investigator
	location  store.Location
	npcs      []store.NPC
	clues     []store.Clue
	items     []store.Item
	eventTail []store.Event
	stage     string
	err       error
}

type sidePanel int

const (
	panelScene sidePanel = iota
	panelPeople
	panelClues
	panelLog
	sidePanelCount
)

func (p sidePanel) label() string {
	switch p {
	case panelPeople:
		return "人物"
	case panelClues:
		return "线索"
	case panelLog:
		return "裁定"
	default:
		return "场景"
	}
}

type overlayMode int

const (
	overlayNone overlayMode = iota
	overlayHelp
	overlayCommands
)

// Model 是 bubbletea 应用状态。
type Model struct {
	runner Runner
	store  *store.Store
	scn    *scenario.Scenario

	save     store.Save
	inv      store.Investigator
	location store.Location
	npcs     []store.NPC
	clues    []store.Clue
	items    []store.Item
	events   []store.Event
	stage    string

	log         []logEntry
	input       textinput.Model
	spinner     spinner.Model
	busy        bool
	endingShown bool
	drift       string
	width       int
	height      int
	quitting    bool

	activePanel sidePanel
	overlay     overlayMode
	storyOffset int
	actionIndex int

	completionBase  string
	completionIndex int

	// ctx 用于 RunTurn；通常是 context.Background()，由 main.go 注入。
	ctx context.Context
}

// New 构造一个 Model。
func New(ctx context.Context, runner Runner, st *store.Store, openingNarrative string, scn ...*scenario.Scenario) Model {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = "描述行动，或 Ctrl+P 打开命令"
	in.Focus()
	in.CharLimit = 1000

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	m := Model{
		runner:  runner,
		store:   st,
		input:   in,
		spinner: sp,
		ctx:     ctx,
	}
	if len(scn) > 0 {
		m.scn = scn[0]
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
			if npcs, err := repo.ListNPCsAtLocation(ctx, sv.ID, sv.CurrentLocationID); err == nil {
				msg.npcs = npcs
			}
		}
		if clues, err := repo.ListFoundClues(ctx, sv.ID); err == nil {
			msg.clues = clues
		}
		if items, err := repo.ListItems(ctx, sv.ID); err == nil {
			msg.items = items
		}
		msg.stage = sv.Stage
		if events, err := repo.ListEvents(ctx, sv.ID, 0, 0); err == nil {
			tail := events
			if len(tail) > 8 {
				tail = tail[len(tail)-8:]
			}
			msg.eventTail = tail
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
		if msg.Type == tea.KeyCtrlC {
			m.quitting = true
			return m, tea.Quit
		}
		if m.overlay != overlayNone {
			switch msg.Type {
			case tea.KeyEsc, tea.KeyEnter, tea.KeyCtrlP:
				m.overlay = overlayNone
				return m, nil
			}
			if len(msg.Runes) == 1 && msg.Runes[0] == '?' {
				m.overlay = overlayNone
				return m, nil
			}
			return m, nil
		}

		switch msg.Type {
		case tea.KeyCtrlP:
			m.overlay = overlayCommands
			return m, nil
		case tea.KeyShiftTab:
			m.activePanel = (m.activePanel + 1) % sidePanelCount
			return m, nil
		case tea.KeyPgUp:
			m.storyOffset += 6
			return m, nil
		case tea.KeyPgDown:
			m.storyOffset = max(0, m.storyOffset-6)
			return m, nil
		}
		if len(msg.Runes) == 1 && msg.Runes[0] == '?' && strings.TrimSpace(m.input.Value()) == "" {
			m.overlay = overlayHelp
			return m, nil
		}

		if m.busy {
			return m, nil
		}
		if strings.TrimSpace(m.input.Value()) == "" {
			if idx, ok := actionDigit(msg); ok {
				return m.fillAction(idx), nil
			}
			switch msg.Type {
			case tea.KeyUp:
				return m.moveAction(-1), nil
			case tea.KeyDown:
				return m.moveAction(1), nil
			}
		}
		switch msg.Type {
		case tea.KeyEsc:
			m.quitting = true
			return m, tea.Quit
		case tea.KeyTab:
			return m.completeInput(), nil
		case tea.KeyEnter:
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				text = m.selectedAction()
			}
			m.input.SetValue("")
			m.resetCompletion()
			if text == "" {
				return m, nil
			}
			return m.handleSubmit(text)
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.resetCompletion()
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
		m.npcs = msg.npcs
		m.clues = msg.clues
		m.items = msg.items
		m.events = msg.eventTail
		m.stage = msg.stage
		if actions := m.actionOptions(); len(actions) == 0 || m.actionIndex >= len(actions) {
			m.actionIndex = 0
		}
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
	m.storyOffset = 0
	return m, tea.Batch(m.runTurnCmd(text), m.spinner.Tick)
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
	m.input.SetValue(actions[idx].input)
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
	return actions[idx].input
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
	return actions[idx].label
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
		m.log = append(m.log, logEntry{kind: EntrySystem, text: "当前时段：" + displayTimeOfDay(m.save.TimeOfDay)})
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
		// /talk <人物>：把后续输入当作"对该人物说话"，MVP 实现为 prefix 注入并立即提交。
		if c.arg == "" {
			m.log = append(m.log, logEntry{kind: EntryError, text: "用法: /talk <人物> <你想说的话>"})
			return m, nil
		}
		text := fmt.Sprintf("[talk:%s] %s", c.arg, c.rest)
		m.log = append(m.log, logEntry{kind: EntryPlayer, text: m.talkDisplayText(c.arg, c.rest)})
		m.busy = true
		return m, tea.Batch(m.runTurnCmd(text), m.spinner.Tick)
	case "all":
		text := "[all] " + c.rest
		m.log = append(m.log, logEntry{kind: EntryPlayer, text: "对在场所有人说：" + c.rest})
		m.busy = true
		return m, tea.Batch(m.runTurnCmd(text), m.spinner.Tick)
	default:
		m.log = append(m.log, logEntry{kind: EntryError, text: "未知命令: /" + c.name + "（/help 查看可用命令）"})
	}
	return m, nil
}

func (m Model) talkDisplayText(target, speech string) string {
	name := target
	for _, npc := range m.npcs {
		if strings.EqualFold(npc.ID, target) || strings.EqualFold(npc.Name, target) {
			name = nonEmpty(npc.Name, target)
			break
		}
	}
	if strings.TrimSpace(speech) == "" {
		return "准备与 " + name + " 交谈。"
	}
	return "对 " + name + " 说：" + speech
}

func (m *Model) appendTurnResult(res orchestrator.TurnResult) {
	if res.Narrative != "" {
		m.log = append(m.log, logEntry{kind: EntryGM, text: res.Narrative})
	}
	for range res.Fired {
		m.log = append(m.log, logEntry{
			kind: EntrySystem,
			text: "故事状态已更新：新的线索或局势变化已记录。",
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
			text: "刚才的叙事和游戏状态可能不完全一致；系统已保留当前可用结果。",
		})
	}
	if res.Ending != nil && !m.endingShown {
		m.endingShown = true
		text := fmt.Sprintf("[%s] %s", strings.ToUpper(res.Ending.Kind), res.Ending.Description)
		if res.Report != nil {
			text = formatCaseReport(res.Report)
		}
		m.log = append(m.log, logEntry{
			kind: EntryEnding,
			text: text,
		})
	}
	m.storyOffset = 0
}

func formatCaseReport(report *scenario.CaseReport) string {
	if report == nil {
		return ""
	}
	lines := []string{
		fmt.Sprintf("[%s] %s", strings.ToUpper(report.Ending.Kind), report.Ending.Description),
	}
	if report.EvidenceStatus != "" {
		lines = append(lines, "", "结案评估", "  "+report.EvidenceStatus)
	}
	if report.CulpritName != "" {
		lines = append(lines, "", "本局真凶", "  "+report.CulpritName)
	}
	if len(report.FoundKeyClues) > 0 {
		lines = append(lines, "", "已掌握关键证据")
		for _, clue := range report.FoundKeyClues {
			lines = append(lines, "  - "+clue.Description)
		}
	}
	if len(report.MissingKeyClues) > 0 {
		lines = append(lines, "", "遗漏关键证据")
		for _, clue := range report.MissingKeyClues {
			lines = append(lines, "  - "+nonEmpty(clue.Description, clue.ID))
		}
	}
	if npcLines := reportNPCOutcomeLines(report.NPCOutcomes); len(npcLines) > 0 {
		lines = append(lines, "", "人物后果")
		lines = append(lines, npcLines...)
	}
	if report.TruthSummary != "" {
		lines = append(lines, "", "本局真相", "  "+report.TruthSummary)
	}
	return strings.Join(lines, "\n")
}

func reportNPCOutcomeLines(outcomes []scenario.NPCOutcome) []string {
	lines := []string{}
	for _, outcome := range outcomes {
		if outcome.Alive && outcome.Relation > -15 {
			continue
		}
		state := "存活"
		if !outcome.Alive {
			state = "死亡"
		}
		lines = append(lines, fmt.Sprintf("  - %s：%s，关系 %+d", nonEmpty(outcome.Name, outcome.ID), state, outcome.Relation))
	}
	return lines
}

// View 渲染整个界面。
func (m Model) View() string {
	if m.quitting {
		return "已退出。\n"
	}

	width := m.width
	if width <= 0 {
		width = 100
	}
	height := m.height
	if height <= 0 {
		height = 30
	}
	width = max(26, width)
	height = max(8, height)

	if width < 72 || height < 16 {
		return m.renderCompactShell(width, height)
	}
	return m.renderWorkbenchShell(width, height)
}

func (m Model) renderWorkbenchShell(width, height int) string {
	briefHeight := 2
	navHeight := 1
	composerHeight := 3
	contentHeight := max(6, height-1-briefHeight-navHeight-composerHeight)
	stage := m.renderMainStage(width, contentHeight)
	if m.overlay != overlayNone {
		stage = m.renderOverlay(width, contentHeight)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderSessionBar(width),
		m.renderBriefing(width, briefHeight),
		m.renderCommandDeck(width, navHeight),
		stage,
		m.renderComposer(width, composerHeight),
	)
}

func (m Model) renderCompactShell(width, height int) string {
	composerHeight := 2
	if height < 12 {
		composerHeight = 1
	}
	contentHeight := max(3, height-1-composerHeight)
	stage := m.renderTimeline(width, contentHeight)
	if m.overlay != overlayNone {
		stage = m.renderOverlay(width, contentHeight)
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderSessionBar(width),
		stage,
		m.renderComposer(width, composerHeight),
	)
}

func (m Model) renderSessionBar(width int) string {
	state := "待行动"
	if m.busy {
		state = "推演中"
	} else if m.endingShown {
		state = "已结局"
	}
	text := fmt.Sprintf("Whisperer 案件桌  回合 %d  %s    %s    %s",
		m.save.TurnCount,
		displayTimeOfDay(m.save.TimeOfDay),
		nonEmpty(m.inv.Name, "调查员"),
		state,
	)
	return sessionBarStyle.Width(width).Render(fitLine(text, width-2))
}

func (m Model) renderBriefing(width, height int) string {
	lines := []string{
		accentStyle.Render("任务简报") + "  " + m.briefLine(),
		accentStyle.Render("建议行动") + "  " + m.primarySuggestion(),
	}
	return fixedLines(lines, width, height)
}

func (m Model) renderCommandDeck(width, height int) string {
	guide := fmt.Sprintf("Ctrl+P 命令 · Shift+Tab 切换案件卡:%s · PgUp/PgDn 回看故事 · ? 帮助", m.activePanel.label())
	return fixedLines([]string{mutedStyle.Render(fitLine(guide, width))}, width, height)
}

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
	if action := m.selectedActionLabel(); action != "" {
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
		out = append(out, choice.label)
	}
	return out
}

func (m Model) actionChoices() []suggestedAction {
	actions := []suggestedAction{}
	if loc, ok := m.scenarioLocation(); ok {
		for _, lead := range loc.Leads {
			lead = strings.TrimSpace(lead)
			if lead != "" {
				actions = append(actions, suggestedAction{label: lead, input: lead})
			}
		}
	}
	if len(actions) > 3 {
		actions = actions[:3]
	}
	actions = append(actions, m.itemActions()...)
	actions = append(actions, m.dialogueActions()...)
	if len(actions) == 0 && m.location.Name != "" {
		text := "观察" + m.location.Name + "，寻找异常痕迹或可调查的物件"
		actions = append(actions, suggestedAction{label: text, input: text})
	}
	if len(actions) == 0 {
		return nil
	}
	if len(actions) > 5 {
		actions = actions[:5]
	}
	return actions
}

func (m Model) dialogueActions() []suggestedAction {
	if m.scn == nil || len(m.npcs) == 0 {
		return nil
	}
	found := m.foundClueSet()
	stage := m.currentStage()
	out := []suggestedAction{}
	for _, npc := range m.npcs {
		snpc, ok := m.scenarioNPC(npc.ID)
		if !ok {
			continue
		}
		for _, opt := range scenario.DialogueOptionsFor(snpc, stage, found) {
			prompt := strings.TrimSpace(opt.Prompt)
			if prompt != "" {
				label := fmt.Sprintf("询问%s：%s", nonEmpty(snpc.Name, npc.Name), opt.Label)
				out = append(out, suggestedAction{label: label, input: prompt})
			}
		}
	}
	return out
}

func (m Model) itemActions() []suggestedAction {
	if m.scn == nil || len(m.items) == 0 {
		return nil
	}
	found := m.foundClueSet()
	stage := m.currentStage()
	out := []suggestedAction{}
	for _, state := range m.items {
		sitem, ok := m.scenarioItem(state.ID)
		if !ok {
			continue
		}
		for _, action := range scenario.ItemActionsFor(sitem, state, stage, m.location.ID, found) {
			label := fmt.Sprintf("使用%s：%s", nonEmpty(sitem.Name, state.Name), action.Label)
			out = append(out, suggestedAction{label: label, input: strings.TrimSpace(action.Prompt)})
		}
	}
	return out
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
	if objective := m.currentObjective(); objective.Title != "" {
		lines = append(lines, "", labelStyle.Render("当前目标"), "  "+objective.Title)
		for i, step := range objective.Steps {
			if i >= 2 {
				break
			}
			lines = append(lines, "  - "+step)
		}
	}
	if actions := m.actionOptions(); len(actions) > 0 {
		lines = append(lines, "", labelStyle.Render("可选行动"))
		for i, action := range actions {
			prefix := fmt.Sprintf("  %d. ", i+1)
			if i == m.actionIndex%len(actions) {
				prefix = accentStyle.Render("› " + fmt.Sprintf("%d. ", i+1))
			}
			lines = append(lines, prefix+action)
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

func (m Model) renderComposer(width, height int) string {
	prompt := "› " + m.input.View()
	if m.busy {
		prompt = m.spinner.View() + " GM 正在推演回合..."
	}
	lines := []string{
		composerStyle.Render(fitLine(prompt, width)),
		mutedStyle.Render(fitLine(m.commandGuide(), width)),
	}
	return fixedLines(lines, width, height)
}

func (m Model) commandGuide() string {
	if strings.HasPrefix(m.input.Value(), "/talk ") {
		names := []string{}
		for _, npc := range m.npcs {
			if npc.ID != "" {
				names = append(names, npc.ID)
			}
		}
		if len(names) > 0 {
			return "Tab 补全目标 · " + strings.Join(names, " · ")
		}
	}
	if strings.TrimSpace(m.input.Value()) == "" && len(m.actionOptions()) > 0 {
		return "↑/↓ 选择行动 · 1-5 填入 · Enter 执行 · Ctrl+P 命令 · ? 帮助"
	}
	return "Enter 发送 · Tab 补全 · /hint 提示 · /talk 对话 · /sheet 状态 · /inv 背包"
}

func (m Model) renderOverlay(width, height int) string {
	switch m.overlay {
	case overlayCommands:
		return renderPinnedPane("命令面板", m.commandOverlayLines(), width, height)
	case overlayHelp:
		return renderPinnedPane("玩家帮助", m.helpOverlayLines(), width, height)
	default:
		return m.renderMainStage(width, height)
	}
}

func (m Model) commandOverlayLines() []string {
	return []string{
		accentStyle.Render("常用行动"),
		"  直接输入自然语言：检查门锁、追问范斯、翻找抽屉。",
		"  /talk <人物> <话>  指名对话，Tab 可补全当前地点人物。",
		"  /all <话>          对全场发言。",
		"",
		accentStyle.Render("调查工具"),
		"  /hint              请求不剧透提示。",
		"  /sheet             查看调查员属性。",
		"  /inv               查看背包。",
		"  /time              查看当前时段。",
		"",
		accentStyle.Render("会话"),
		"  /bind <名字> <职业>  结局后绑定新调查员。",
		"  /quit              退出。",
		"",
		mutedStyle.Render("Esc / Enter / Ctrl+P 关闭命令面板"),
	}
}

func (m Model) helpOverlayLines() []string {
	return []string{
		accentStyle.Render("桌面布局"),
		"  左侧是故事卷轴，保留玩家行动与 GM 叙事。",
		"  右侧案件卡按场景、人物、线索、裁定组织信息。",
		"  下方输入框只负责下一步行动，避免把命令说明挤进正文。",
		"",
		accentStyle.Render("快捷键"),
		"  Ctrl+P     打开命令面板。",
		"  Shift+Tab  切换案件卡标签页。",
		"  PgUp/PgDn  回看或回到最新故事。",
		"  Tab        补全斜杠命令或 NPC 目标。",
		"  Esc        退出；覆盖层打开时先关闭覆盖层。",
		"",
		accentStyle.Render("推荐玩法"),
		"  用自然语言描述意图，不必猜系统命令。",
		"  需要明确目标时使用 /talk；卡住时使用 /hint。",
		"",
		mutedStyle.Render("Esc / Enter / ? 关闭帮助"),
	}
}

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

func summarizeError(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if text == "" {
		return "发生未知错误"
	}

	parts := []string{}
	if strings.Contains(text, "401") || strings.Contains(strings.ToLower(text), "unauthorized") {
		parts = append(parts, "模型调用失败", "401 Unauthorized")
		if strings.Contains(strings.ToLower(text), "x-api-key") {
			parts = append(parts, "invalid x-api-key")
		}
	} else if idx := strings.Index(text, ":"); idx > 0 && idx < 24 {
		parts = append(parts, strings.TrimSpace(text[:idx]))
		rest := strings.TrimSpace(text[idx+1:])
		if rest != "" {
			parts = append(parts, firstSentence(rest))
		}
	} else {
		parts = append(parts, firstSentence(text))
	}

	if requestID := extractRequestID(text); requestID != "" {
		parts = append(parts, "request "+requestID)
	}
	return strings.Join(parts, " · ")
}

func firstSentence(text string) string {
	for _, sep := range []string{". ", "。", "\n"} {
		if idx := strings.Index(text, sep); idx >= 0 {
			return strings.TrimSpace(text[:idx])
		}
	}
	return text
}

func extractRequestID(text string) string {
	for _, marker := range []string{"Request-ID:", "request_id\":\"", "request_id=", "req_"} {
		idx := strings.Index(text, marker)
		if idx < 0 {
			continue
		}
		if marker == "req_" {
			return readToken(text[idx:])
		}
		return readToken(text[idx+len(marker):])
	}
	return ""
}

func readToken(text string) string {
	text = strings.TrimLeft(text, " \t\"'")
	var b strings.Builder
	for _, r := range text {
		if r == '"' || r == '\'' || r == ')' || r == '}' || r == ',' || r == ' ' || r == '\t' || r == '\n' {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
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

func renderEntry(e logEntry) string {
	switch e.kind {
	case EntryGM:
		return gmStyle.Render("GM  ") + e.text
	case EntryPlayer:
		return playerStyle.Render("你  ") + e.text
	case EntrySystem:
		return systemStyle.Render("系统 ") + e.text
	case EntryError:
		return errorStyle.Render("错误 ") + e.text
	case EntryEnding:
		return endingStyle.Render("结局 ") + e.text
	}
	return e.text
}

func helpText() string {
	return strings.Join([]string{
		"命令",
		"/sheet                 显示调查员属性",
		"/inventory (/inv)      显示背包",
		"/time                  显示当前时段",
		"/talk <人物> <话>      指名对某人说话",
		"/all <话>              对全场说话",
		"/hint                  请求一个不剧透的提示",
		"/bind <name> <职业>    在结局后绑定新调查员到本剧本",
		"/help                  显示本帮助",
		"/quit (/exit /q)       退出",
	}, "\n")
}

func nonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func displayTimeOfDay(t store.TimeOfDay) string {
	switch t {
	case store.TimeMorning:
		return "清晨"
	case store.TimeAfternoon:
		return "午后"
	case store.TimeNight:
		return "夜晚"
	default:
		return "未知时段"
	}
}

func displayStage(stage string) string {
	switch strings.ToLower(strings.TrimSpace(stage)) {
	case "opening", "act1":
		return "开局"
	case "investigation", "act2":
		return "调查"
	case "confrontation", "act3":
		return "对峙"
	default:
		return nonEmpty(stage, "开局")
	}
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
		ID:         "inv-" + name,
		Name:       name,
		Occupation: occupation,
		AttrsJSON:  `{"STR":50,"CON":60,"SIZ":55,"DEX":60,"APP":50,"INT":75,"POW":60,"EDU":80}`,
		SkillsJSON: `{"Spot Hidden":50,"Library Use":60,"Listen":40,"Psychology":40}`,
		HP:         12, MP: 12, SAN: 60,
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
