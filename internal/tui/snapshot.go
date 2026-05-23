package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

// snapshotMsg 是初始状态加载（save / investigator / location）完成的消息。
type snapshotMsg struct {
	save      store.Save
	inv       store.Investigator
	location  store.Location
	npcs      []store.NPC
	clues     []store.Clue
	items     []store.Item
	threats   []scenario.ThreatStatus
	eventTail []store.Event
	fired     map[string]bool
	stage     string
	err       error
}

// turnDoneMsg 是 RunTurn 完成后的异步消息。
type turnDoneMsg struct {
	res orchestrator.TurnResult
	err error
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
		if m.scn != nil {
			engine := scenario.New(m.scn, repo, nil)
			if threats, err := engine.Threats(ctx, sv.ID); err == nil {
				msg.threats = threats
			}
		}
		msg.stage = sv.Stage
		if events, err := repo.ListEvents(ctx, sv.ID, 0, 0); err == nil {
			msg.fired = firedTriggerSet(events)
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
