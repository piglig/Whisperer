package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

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

type suggestedAction = orchestrator.SuggestedAction

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
	threats  []scenario.ThreatStatus
	events   []store.Event
	fired    map[string]bool
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
		m.threats = msg.threats
		m.events = msg.eventTail
		m.fired = msg.fired
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
