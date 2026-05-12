package tui

import (
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

func TestModel_TimeCommand(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	cmd := m.loadSnapshotCmd()
	updated, _ := m.Update(cmd())
	mm := updated.(Model)
	mm.input.SetValue("/time")
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = updated.(Model)
	assert.Contains(t, mm.log[len(mm.log)-1].text, "morning")
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
