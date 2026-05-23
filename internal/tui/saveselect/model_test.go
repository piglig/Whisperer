package saveselect

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/store"
)

type fakeDeleter struct {
	deleted []string
	err     error
}

func (f *fakeDeleter) DeleteSave(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	f.deleted = append(f.deleted, id)
	return nil
}

func makeSaves() []store.Save {
	return []store.Save{
		{ID: "save-alpha", Name: "untitled", ScenarioID: "fog_harbor", VariantID: "vance_executes", TurnCount: 7, TimeOfDay: store.TimeNight, UpdatedAt: 1700000000000},
		{ID: "save-bravo", Name: "untitled", ScenarioID: "fog_harbor", VariantID: "calvin_directs", TurnCount: 2, TimeOfDay: store.TimeMorning, UpdatedAt: 1690000000000},
	}
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func updateN(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func TestEnterLoadsHighlightedSave(t *testing.T) {
	m := New(makeSaves(), nil, nil)
	m = updateN(m, keyMsg("enter"))
	assert.True(t, m.quitting)
	assert.Equal(t, Result{Action: ActionLoad, SaveID: "save-alpha"}, m.Result())
}

func TestArrowDownSelectsNextSaveThenNewRow(t *testing.T) {
	m := New(makeSaves(), nil, nil)
	m = updateN(m, keyMsg("down"))
	m = updateN(m, keyMsg("enter"))
	assert.Equal(t, "save-bravo", m.Result().SaveID)

	m2 := New(makeSaves(), nil, nil)
	m2 = updateN(m2, keyMsg("down"), keyMsg("down"), keyMsg("enter"))
	assert.Equal(t, ActionNew, m2.Result().Action)
	assert.Empty(t, m2.Result().SaveID)
}

func TestArrowDownDoesNotExceedRows(t *testing.T) {
	m := New(makeSaves(), nil, nil)
	// 2 saves + 1 new row → cursor ∈ [0,2]. 多按几次 down 不应越界。
	m = updateN(m, keyMsg("down"), keyMsg("down"), keyMsg("down"), keyMsg("down"))
	assert.Equal(t, 2, m.cursor)
}

func TestNKeyChoosesNewSave(t *testing.T) {
	m := New(makeSaves(), nil, nil)
	m = updateN(m, keyMsg("n"))
	assert.Equal(t, ActionNew, m.Result().Action)
}

func TestQuitKeys(t *testing.T) {
	for _, k := range []string{"q", "esc", "ctrl+c"} {
		m := New(makeSaves(), nil, nil)
		m = updateN(m, keyMsg(k))
		assert.Equal(t, ActionQuit, m.Result().Action, "key=%s", k)
	}
}

func TestDeleteRequiresDoubleConfirm(t *testing.T) {
	fd := &fakeDeleter{}
	cleaned := []string{}
	m := New(makeSaves(), fd, func(id string) error {
		cleaned = append(cleaned, id)
		return nil
	})

	// 第一次 d → 进入待确认状态，不删
	m = updateN(m, keyMsg("d"))
	assert.Equal(t, 0, m.pendingDelete)
	assert.Empty(t, fd.deleted)

	// 第二次 d → 实删
	m = updateN(m, keyMsg("d"))
	assert.Equal(t, []string{"save-alpha"}, fd.deleted)
	assert.Equal(t, []string{"save-alpha"}, cleaned)
	assert.Len(t, m.saves, 1)
	assert.Equal(t, -1, m.pendingDelete)
}

func TestDeleteCancelOnOtherKey(t *testing.T) {
	fd := &fakeDeleter{}
	m := New(makeSaves(), fd, nil)
	m = updateN(m, keyMsg("d"))
	require.Equal(t, 0, m.pendingDelete)

	// 切到下一行应取消 pending
	m = updateN(m, keyMsg("down"))
	assert.Equal(t, -1, m.pendingDelete)
	assert.Empty(t, fd.deleted)
}

func TestDeleteOnNewRowIsNoop(t *testing.T) {
	fd := &fakeDeleter{}
	m := New(makeSaves(), fd, nil)
	m = updateN(m, keyMsg("down"), keyMsg("down")) // 到 "+ 新建" 行
	m = updateN(m, keyMsg("d"), keyMsg("d"))
	assert.Empty(t, fd.deleted)
	assert.Equal(t, -1, m.pendingDelete)
}

func TestDeleteErrorSurfacesInStatus(t *testing.T) {
	fd := &fakeDeleter{err: errors.New("locked")}
	m := New(makeSaves(), fd, nil)
	m = updateN(m, keyMsg("d"), keyMsg("d"))
	assert.Contains(t, m.status, "locked")
	assert.Len(t, m.saves, 2) // 没删成
}

func TestCleanupErrorStillRemovesFromList(t *testing.T) {
	fd := &fakeDeleter{}
	m := New(makeSaves(), fd, func(_ string) error { return errors.New("rmdir fail") })
	m = updateN(m, keyMsg("d"), keyMsg("d"))
	assert.Len(t, m.saves, 1)
	assert.Contains(t, m.status, "清理失败")
}

func TestDeleteLastSaveLandsOnNewRow(t *testing.T) {
	fd := &fakeDeleter{}
	m := New(makeSaves()[:1], fd, nil)
	m = updateN(m, keyMsg("d"), keyMsg("d"))
	assert.Empty(t, m.saves)
	assert.Equal(t, 0, m.cursor) // newRowIndex
	assert.Contains(t, m.status, "已无存档")

	// Enter 应该走 ActionNew
	m = updateN(m, keyMsg("enter"))
	assert.Equal(t, ActionNew, m.Result().Action)
}

func TestViewRendersSavesAndCursor(t *testing.T) {
	m := New(makeSaves(), nil, nil)
	out := m.View()
	assert.Contains(t, out, "选择存档")
	assert.Contains(t, out, "fog_harbor")
	assert.Contains(t, out, "vance_executes")
	assert.Contains(t, out, "第 7 回合")
	assert.Contains(t, out, "+ 新建存档")
	// 光标在第一行
	firstSaveLine := strings.Index(out, "fog_harbor")
	assert.Greater(t, firstSaveLine, 0)
}

func TestViewShowsPendingDeleteHint(t *testing.T) {
	m := New(makeSaves(), &fakeDeleter{}, nil)
	m = updateN(m, keyMsg("d"))
	out := m.View()
	assert.Contains(t, out, "再按 d 确认")
}

func TestQuittingViewIsEmpty(t *testing.T) {
	m := New(makeSaves(), nil, nil)
	m = updateN(m, keyMsg("q"))
	assert.Empty(t, m.View())
}
