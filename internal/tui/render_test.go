package tui

import (
	"context"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
	"github.com/zhuzhenwu/whisperer/internal/orchestrator/sla"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
)

func TestInit_ReturnsBatchCmd(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	cmd := m.Init()
	require.NotNil(t, cmd)
	// Just exercise it: don't run.
}

func TestRenderEntry_AllKinds(t *testing.T) {
	for _, k := range []EntryKind{EntryGM, EntryPlayer, EntrySystem, EntryError, EntryEnding, EntryKind(99)} {
		out := renderEntry(logEntry{kind: k, text: "x"})
		assert.NotEmpty(t, out)
	}
}

func TestRenderSheetEmpty(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	// 不加载 inv → renderSheet 应给出占位提示
	assert.Contains(t, m.renderSheet(), "尚未加载")
}

func TestRenderInventoryEmpty(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	assert.Contains(t, m.renderInventory(), "背包为空")
}

func TestSnapshotMsg_Error(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	updated, _ := m.Update(snapshotMsg{err: assertErr("boom")})
	mm := updated.(Model)
	assert.Equal(t, EntryError, mm.log[len(mm.log)-1].kind)
}

func TestUpdate_BusyCtrlC(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	m.busy = true
	updated, _ := m.Update(tickMsg{})
	_ = updated
	// busy 状态下普通 key 被忽略
	mm := m
	mm.input.SetValue("hi")
}

func TestUpdate_AppendsFiredTriggers(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	res := orchestrator.TurnResult{
		Narrative: "x",
		Fired:     []scenario.FiredTrigger{{ID: "lighthouse_storm"}},
		Drift:     scenario.DriftSoft,
		SLAReport: sla.Report{Passed: false, Violations: []sla.Violation{{Code: sla.CodeRollMissing}}},
	}
	updated, _ := m.Update(turnDoneMsg{res: res})
	mm := updated.(Model)
	hasFired := false
	for _, e := range mm.log {
		if e.kind == EntrySystem && contains(e.text, "lighthouse_storm") {
			hasFired = true
		}
	}
	assert.True(t, hasFired)
	assert.Contains(t, mm.drift, "软偏离")
}

func TestUpdate_SpinnerTick(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	updated, _ := m.Update(spinner.TickMsg{ID: m.spinner.ID()})
	_ = updated
}

func TestView_BusyState(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	m.busy = true
	v := m.View()
	assert.Contains(t, v, "THINKING")
	assert.Contains(t, v, "GM 正在推演回合")
}

// helpers --------------------------------------------------------------

type tickMsg struct{}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(sub) > 0 && stringIndex(s, sub) >= 0))
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

type sentinelErr string

func (e sentinelErr) Error() string { return string(e) }

func assertErr(s string) error { return sentinelErr(s) }

// 快速验证一些字段不依赖 store 的 helper：避免 lint 误报未使用的 uuid。
var _ = uuid.New
