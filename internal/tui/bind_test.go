package tui

import (
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

// fakeBinder 实现 Runner + Binder。
type fakeBinder struct {
	saveID    string
	res       orchestrator.TurnResult
	bindCalls []store.Investigator
	bindErr   error
}

func (f *fakeBinder) RunTurn(_ context.Context, _ string) (orchestrator.TurnResult, error) {
	return f.res, nil
}
func (f *fakeBinder) SaveID() string { return f.saveID }
func (f *fakeBinder) BindNewInvestigator(_ context.Context, inv store.Investigator) error {
	f.bindCalls = append(f.bindCalls, inv)
	return f.bindErr
}

func TestBind_RequiresEndingShown(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeBinder{saveID: id}
	m := New(context.Background(), r, st, "")
	m.input.SetValue("/bind Theo 教士")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	assert.Equal(t, EntryError, mm.log[len(mm.log)-1].kind)
	assert.Empty(t, r.bindCalls)
}

func TestBind_HappyPath(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeBinder{saveID: id}
	m := New(context.Background(), r, st, "")
	// 先制造一个 ending
	updated, _ := m.Update(turnDoneMsg{res: orchestrator.TurnResult{
		Ending: &scenario.Ending{ID: "x", Kind: "failure", Description: "结束"},
	}})
	mm := updated.(Model)
	require.True(t, mm.endingShown)

	mm.input.SetValue("/bind Theo 教士")
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = updated.(Model)
	require.Len(t, r.bindCalls, 1)
	assert.Equal(t, "Theo", r.bindCalls[0].Name)
	assert.Equal(t, "教士", r.bindCalls[0].Occupation)
	assert.False(t, mm.endingShown, "endingShown 应被重置以允许继续游戏")
}

func TestBind_DefaultsOccupation(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeBinder{saveID: id}
	m := New(context.Background(), r, st, "")
	updated, _ := m.Update(turnDoneMsg{res: orchestrator.TurnResult{
		Ending: &scenario.Ending{ID: "x", Kind: "failure", Description: "结束"},
	}})
	mm := updated.(Model)

	mm.input.SetValue("/bind Solo")
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.Len(t, r.bindCalls, 1)
	assert.Equal(t, "调查员", r.bindCalls[0].Occupation, "缺职业时应填默认值")
	_ = updated
}

func TestBind_NoArg(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeBinder{saveID: id}
	m := New(context.Background(), r, st, "")
	updated, _ := m.Update(turnDoneMsg{res: orchestrator.TurnResult{
		Ending: &scenario.Ending{ID: "x", Kind: "failure", Description: "结束"},
	}})
	mm := updated.(Model)

	mm.input.SetValue("/bind")
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = updated.(Model)
	assert.Equal(t, EntryError, mm.log[len(mm.log)-1].kind)
	assert.Empty(t, r.bindCalls)
}

func TestBind_BinderError(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeBinder{saveID: id, bindErr: errors.New("upstream")}
	m := New(context.Background(), r, st, "")
	updated, _ := m.Update(turnDoneMsg{res: orchestrator.TurnResult{
		Ending: &scenario.Ending{ID: "x", Kind: "failure", Description: "结束"},
	}})
	mm := updated.(Model)
	mm.input.SetValue("/bind Theo")
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = updated.(Model)
	assert.Equal(t, EntryError, mm.log[len(mm.log)-1].kind)
}

func TestBind_RunnerWithoutBinder(t *testing.T) {
	st, id := newTestStore(t)
	// fakeRunner 不实现 Binder
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	updated, _ := m.Update(turnDoneMsg{res: orchestrator.TurnResult{
		Ending: &scenario.Ending{ID: "x", Kind: "failure", Description: "结束"},
	}})
	mm := updated.(Model)
	mm.input.SetValue("/bind Theo")
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = updated.(Model)
	assert.Equal(t, EntryError, mm.log[len(mm.log)-1].kind)
	assert.Contains(t, mm.log[len(mm.log)-1].text, "不可用")
}

func TestHint_TriggersRunTurn(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeBinder{saveID: id, res: orchestrator.TurnResult{Narrative: "微弱的暗示。"}}
	m := New(context.Background(), r, st, "")
	m.input.SetValue("/hint")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	assert.True(t, mm.busy)
	// 系统提示已记入日志
	hasHintEntry := false
	for _, e := range mm.log {
		if e.kind == EntrySystem && len(e.text) > 0 && e.text == "（已请求 GM 提示）" {
			hasHintEntry = true
		}
	}
	assert.True(t, hasHintEntry)
}
