package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

// fakeRunner 假装一个 orchestrator，用于 Model 单测。
type fakeRunner struct {
	saveID string
	res    orchestrator.TurnResult
	err    error
	calls  []string
}

func (f *fakeRunner) RunTurn(_ context.Context, input string) (orchestrator.TurnResult, error) {
	f.calls = append(f.calls, input)
	return f.res, f.err
}

func (f *fakeRunner) SaveID() string { return f.saveID }

func newTestStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	id := uuid.NewString()
	require.NoError(t, s.Repo().CreateSave(ctx, store.Save{ID: id, Name: "x", ScenarioID: "fh"}))
	require.NoError(t, s.Repo().UpsertInvestigator(ctx, store.Investigator{
		ID: "inv", SaveID: id, Name: "Lyra", Occupation: "记者",
		AttrsJSON: "{}", SkillsJSON: "{}", InventoryJSON: `["pen"]`,
		HP: 12, MP: 10, SAN: 60, Active: true,
	}))
	require.NoError(t, s.Repo().UpsertLocation(ctx, store.Location{
		ID: "harbor", SaveID: id, Name: "雾港", Description: "x",
	}))
	require.NoError(t, s.Repo().UpsertNPC(ctx, store.NPC{
		ID: "vance", SaveID: id, Name: "范斯", Personality: "冷静",
		KnowledgeJSON: "{}", LocationID: "harbor", Alive: true,
	}))
	require.NoError(t, s.Repo().UpsertNPC(ctx, store.NPC{
		ID: "villager", SaveID: id, Name: "村民", Personality: "紧张",
		KnowledgeJSON: "{}", LocationID: "harbor", Alive: true,
	}))
	require.NoError(t, s.Repo().UpsertClue(ctx, store.Clue{
		ID: "cloth", SaveID: id, ScenarioID: "fh", Description: "湿布片",
		Found: true, FoundInLocationID: "harbor", FoundAtTurn: 1,
	}))
	require.NoError(t, s.Repo().UpdateSaveProgress(ctx, id, "harbor", 0))
	return s, id
}

func TestModel_OpeningRendered(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeRunner{saveID: id}
	m := New(context.Background(), r, st, "开场白")
	v := m.View()
	assert.Contains(t, v, "开场白")
	assert.Contains(t, v, "Whisperer")
	assert.Contains(t, v, "任务简报")
	assert.Contains(t, v, "行动流")
	assert.Contains(t, v, "案件卡")
}

func TestModel_SnapshotMsgPopulatesState(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeRunner{saveID: id}
	m := New(context.Background(), r, st, "")
	cmd := m.loadSnapshotCmd()
	msg := cmd()
	updated, _ := m.Update(msg)
	mm := updated.(Model)
	assert.Equal(t, "Lyra", mm.inv.Name)
	assert.Equal(t, "雾港", mm.location.Name)
	assert.Len(t, mm.npcs, 2)
	assert.Len(t, mm.clues, 1)
	assert.Equal(t, store.TimeMorning, mm.save.TimeOfDay)
}

func TestModel_HandleSlashHelp(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeRunner{saveID: id}
	m := New(context.Background(), r, st, "")
	m.input.SetValue("/help")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	last := mm.log[len(mm.log)-1]
	assert.Equal(t, EntrySystem, last.kind)
	assert.Contains(t, last.text, "/sheet")
}

func TestModel_HandleSlashSheetAndInventory(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeRunner{saveID: id}
	m := New(context.Background(), r, st, "")
	// 注入 inventory
	cmd := m.loadSnapshotCmd()
	updated, _ := m.Update(cmd())
	mm := updated.(Model)

	mm.input.SetValue("/sheet")
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = updated.(Model)
	assert.Contains(t, mm.log[len(mm.log)-1].text, "Lyra")

	mm.input.SetValue("/inv")
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = updated.(Model)
	assert.Contains(t, mm.log[len(mm.log)-1].text, "pen")
}

func TestModel_HandleSlashUnknown(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	m.input.SetValue("/unknown")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	assert.Equal(t, EntryError, mm.log[len(mm.log)-1].kind)
}

func TestModel_HandleSlashTalkRequiresArg(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	m.input.SetValue("/talk")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	assert.Equal(t, EntryError, mm.log[len(mm.log)-1].kind)
}

func TestModel_PlainInputTriggersRunTurn(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeRunner{
		saveID: id,
		res: orchestrator.TurnResult{
			Narrative: "你环顾四周。",
			Drift:     scenario.DriftOK,
		},
	}
	m := New(context.Background(), r, st, "")
	m.input.SetValue("look around")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	assert.True(t, mm.busy)

	// 直接调一次 runTurnCmd 拿到 turnDoneMsg，避开 tea.Batch 包装。
	msg := mm.runTurnCmd("look around")()
	updated, _ = mm.Update(msg)
	mm = updated.(Model)
	assert.False(t, mm.busy)
	assert.Contains(t, mm.log[len(mm.log)-1].text, "你环顾四周")
	assert.NotEmpty(t, r.calls)
}

func TestModel_RunTurnError(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeRunner{saveID: id, err: errors.New("boom")}
	m := New(context.Background(), r, st, "")
	updated, _ := m.Update(turnDoneMsg{err: errors.New("boom")})
	mm := updated.(Model)
	assert.Equal(t, EntryError, mm.log[len(mm.log)-1].kind)
	_ = r
}

func TestModel_TurnResultEnding(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	res := orchestrator.TurnResult{
		Narrative: "终。",
		Ending:    &scenario.Ending{ID: "victim_dies", Kind: "failure", Description: "失败"},
		Drift:     scenario.DriftHard,
	}
	updated, _ := m.Update(turnDoneMsg{res: res})
	mm := updated.(Model)
	assert.True(t, mm.endingShown)
	assert.Contains(t, mm.drift, "硬偏离")
	hasEnd := false
	for _, e := range mm.log {
		if e.kind == EntryEnding {
			hasEnd = true
		}
	}
	assert.True(t, hasEnd)
}

func TestModel_QuitOnCtrlC(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	mm := updated.(Model)
	assert.True(t, mm.quitting)
}

func TestModel_BusyIgnoresEnter(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	m.busy = true
	m.input.SetValue("hi")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	assert.True(t, mm.busy)
	// 仍然 busy；不应触发新一回合
}

func TestModel_WindowResize(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	mm := updated.(Model)
	assert.Equal(t, 100, mm.width)
	assert.Equal(t, 40, mm.height)
}

func TestModel_ViewKeepsTerminalHeightWithLongHistory(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "开场白")
	updated, _ := m.Update(m.loadSnapshotCmd()())
	m = updated.(Model)
	m.width = 82
	m.height = 18
	for i := 0; i < 30; i++ {
		m.log = append(m.log,
			logEntry{kind: EntryPlayer, text: "我继续调查码头仓库里一段很长很长很长的描述，用来模拟玩家连续输入命令。"},
			logEntry{kind: EntryGM, text: "GM 返回一段很长很长很长的叙事文本，包含状态、地点、线索和后续行动建议。"},
			logEntry{kind: EntryError, text: "模型调用失败: 这是一段很长很长很长的错误信息，不能把右侧面板撑破。"},
		)
	}

	v := m.View()

	assert.LessOrEqual(t, lipgloss.Height(v), 18)
	for _, line := range strings.Split(v, "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 82)
	}
}

func TestModel_ViewCollapsesNarrativeHistory(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	m.width = 100
	m.height = 28
	for i := 0; i < 18; i++ {
		m.log = append(m.log, logEntry{kind: EntryGM, text: "叙事段落"})
	}

	v := m.View()

	assert.Contains(t, v, "之前 4 条行动已折叠")
}

func TestModel_ViewSummarizesProviderErrors(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	m.width = 100
	m.height = 28
	raw := `回合失败: gm respond: agent: LLM call failed at iter 1: POST "https://api.anthropic.com/v1/messages": 401 Unauthorized {"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"},"request_id":"req_011CbFaau68bKZM"}`
	m.log = append(m.log, logEntry{kind: EntryError, text: raw})

	v := m.View()
	summary := summarizeError(raw)

	assert.Contains(t, summary, "模型调用失败")
	assert.Contains(t, summary, "401 Unauthorized")
	assert.Contains(t, summary, "invalid x-api-key")
	assert.Contains(t, v, "模型调用失败")
	assert.NotContains(t, v, "https://api.anthropic.com")
}

func TestModel_ViewUsesWorkbenchLayoutWithCaseRail(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "开场白")
	updated, _ := m.Update(m.loadSnapshotCmd()())
	m = updated.(Model)
	m.width = 120
	m.height = 30
	m.input.SetValue("/talk ")

	v := m.View()

	assert.Contains(t, v, "Whisperer 案件桌")
	assert.Contains(t, v, "任务简报")
	assert.Contains(t, v, "建议行动")
	assert.Contains(t, v, "行动流")
	assert.Contains(t, v, "案件卡")
	assert.Contains(t, v, "地点")
	assert.Contains(t, v, "在场人物")
	assert.Contains(t, v, "/talk vance")
	assert.Contains(t, v, "Tab 补全目标")
	assert.NotContains(t, v, "case:fh")
}

func TestModel_CommandAndHelpOverlays(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "开场白")
	m.width = 100
	m.height = 28

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	mm := updated.(Model)
	assert.Equal(t, overlayCommands, mm.overlay)
	assert.Contains(t, mm.View(), "命令面板")
	assert.Contains(t, mm.View(), "指名对话")

	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = updated.(Model)
	assert.Equal(t, overlayNone, mm.overlay)

	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	mm = updated.(Model)
	assert.Equal(t, overlayHelp, mm.overlay)
	assert.Contains(t, mm.View(), "玩家帮助")
	assert.Contains(t, mm.View(), "Shift+Tab")
}

func TestModel_CaseRailTabsCycle(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "开场白")
	updated, _ := m.Update(m.loadSnapshotCmd()())
	m = updated.(Model)
	m.width = 120
	m.height = 30

	assert.Contains(t, m.View(), "地点")

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = updated.(Model)
	assert.Equal(t, panelPeople, m.activePanel)
	assert.Contains(t, m.View(), "人物")
	assert.Contains(t, m.View(), "/talk vance")

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = updated.(Model)
	assert.Equal(t, panelClues, m.activePanel)
	assert.Contains(t, m.View(), "线索")
	assert.Contains(t, m.View(), "湿布片")
}

func TestModel_StoryScrollsWithPageKeys(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	m.width = 100
	m.height = 18
	for i := 0; i < 24; i++ {
		m.log = append(m.log, logEntry{kind: EntryGM, text: "叙事段落"})
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	mm := updated.(Model)
	assert.Greater(t, mm.storyOffset, 0)
	assert.Contains(t, mm.View(), "↑ 更早内容")

	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	mm = updated.(Model)
	assert.Zero(t, mm.storyOffset)
}

func TestModel_ViewHidesCommandHintOnShortScreens(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "开场白")
	m.width = 36
	m.height = 10

	v := m.View()

	assert.LessOrEqual(t, lipgloss.Height(v), 10)
	assert.NotContains(t, v, "Enter 发送")
	for _, line := range strings.Split(v, "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 36)
	}
}

func TestModel_TimeCommand(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	cmd := m.loadSnapshotCmd()
	updated, _ := m.Update(cmd())
	mm := updated.(Model)
	mm.input.SetValue("/time")
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = updated.(Model)
	assert.Contains(t, mm.log[len(mm.log)-1].text, "清晨")
}

func TestModel_AllCommandRunsTurn(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeRunner{saveID: id, res: orchestrator.TurnResult{Narrative: "众人沉默。"}}
	m := New(context.Background(), r, st, "")
	m.input.SetValue("/all 我是侦探")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	assert.True(t, mm.busy)
	// 直接跑 runTurnCmd 还原 RunTurn 调用
	msg := mm.runTurnCmd("[all] 我是侦探")()
	updated, _ = mm.Update(msg)
	mm = updated.(Model)
	require.NotEmpty(t, r.calls)
	assert.Contains(t, r.calls[0], "[all]")
	assert.Contains(t, mm.log[len(mm.log)-1].text, "众人沉默")
	assert.NotContains(t, mm.log[0].text, "[all]")
	assert.Contains(t, mm.log[0].text, "对在场所有人说")
}

func TestModel_TalkCommandRunsTurn(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeRunner{saveID: id, res: orchestrator.TurnResult{Narrative: "范斯说话。"}}
	m := New(context.Background(), r, st, "")
	m.input.SetValue("/talk vance 你看到什么了？")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	assert.True(t, mm.busy)
	msg := mm.runTurnCmd("[talk:vance] 你看到什么了？")()
	updated, _ = mm.Update(msg)
	require.NotEmpty(t, r.calls)
	assert.Contains(t, r.calls[0], "[talk:vance]")
	assert.NotContains(t, mm.log[0].text, "[talk:vance]")
	assert.Contains(t, mm.log[0].text, "对 vance 说")
	_ = updated
}

func TestModel_QuitCommand(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	m.input.SetValue("/quit")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	assert.True(t, mm.quitting)
	assert.Contains(t, mm.View(), "已退出")
}

func TestModel_TabCompletesCommand(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	m.input.SetValue("/hi")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	mm := updated.(Model)

	assert.Equal(t, "/hint", mm.input.Value())
}

func TestModel_TabCompletesTalkNPC(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	updated, _ := m.Update(m.loadSnapshotCmd()())
	mm := updated.(Model)
	mm.input.SetValue("/talk va")

	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyTab})
	mm = updated.(Model)

	assert.Equal(t, "/talk vance", mm.input.Value())
}

func TestModel_TabCyclesTalkNPCs(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	updated, _ := m.Update(m.loadSnapshotCmd()())
	mm := updated.(Model)
	mm.input.SetValue("/talk v")

	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyTab})
	mm = updated.(Model)
	first := mm.input.Value()
	mm.input.SetValue("/talk v")
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyTab})
	mm = updated.(Model)
	second := mm.input.Value()

	assert.NotEqual(t, first, second)
	assert.Contains(t, []string{first, second}, "/talk vance")
	assert.Contains(t, []string{first, second}, "/talk villager")
}
