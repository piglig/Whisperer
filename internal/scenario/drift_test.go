package scenario

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/store"
)

func newDriftEnv(t *testing.T) (*Detector, *store.Repository, string, context.Context) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	r := s.Repo()
	saveID := uuid.NewString()
	require.NoError(t, r.CreateSave(ctx, store.Save{ID: saveID, Name: "t", ScenarioID: "fh"}))
	return NewDetector(&Scenario{}, r), r, saveID, ctx
}

func TestDrift_OK_AfterProgress(t *testing.T) {
	d, r, saveID, ctx := newDriftEnv(t)
	_, err := r.AppendEvent(ctx, store.Event{SaveID: saveID, Turn: 3, Type: FiredEventType, Description: "tx"})
	require.NoError(t, err)
	st, err := d.Tick(ctx, saveID, 3)
	require.NoError(t, err)
	assert.Equal(t, DriftOK, st)
	assert.Equal(t, "ok", st.String())
}

func TestDrift_SoftAfterTwoStaleTurns(t *testing.T) {
	d, r, saveID, ctx := newDriftEnv(t)
	_, _ = r.AppendEvent(ctx, store.Event{SaveID: saveID, Turn: 1, Type: FiredEventType, Description: "tx"})
	st, err := d.Tick(ctx, saveID, 3)
	require.NoError(t, err)
	assert.Equal(t, DriftSoft, st)
	assert.Equal(t, "soft", st.String())
}

func TestDrift_HardAfterThreeStaleTurns(t *testing.T) {
	d, r, saveID, ctx := newDriftEnv(t)
	_, _ = r.AppendEvent(ctx, store.Event{SaveID: saveID, Turn: 1, Type: "scenario", Description: "key"})
	st, err := d.Tick(ctx, saveID, 4)
	require.NoError(t, err)
	assert.Equal(t, DriftHard, st)
	assert.Equal(t, "hard", st.String())
}

func TestDrift_NoEventsTreatedAsStale(t *testing.T) {
	d, _, saveID, ctx := newDriftEnv(t)
	st, err := d.Tick(ctx, saveID, 3)
	require.NoError(t, err)
	assert.Equal(t, DriftHard, st, "0 progress + turn 3 should be hard")
}

func TestDrift_ClueFoundCountsAsProgress(t *testing.T) {
	d, r, saveID, ctx := newDriftEnv(t)
	_, _ = r.AppendEvent(ctx, store.Event{SaveID: saveID, Turn: 5, Type: store.EventNarrative, Description: "clue_found:letter"})
	st, err := d.Tick(ctx, saveID, 5)
	require.NoError(t, err)
	assert.Equal(t, DriftOK, st)
}

func TestDrift_SetThresholds(t *testing.T) {
	d, r, saveID, ctx := newDriftEnv(t)
	d.SetThresholds(5, 10)
	_, _ = r.AppendEvent(ctx, store.Event{SaveID: saveID, Turn: 1, Type: FiredEventType, Description: "tx"})
	st, _ := d.Tick(ctx, saveID, 4)
	assert.Equal(t, DriftOK, st)
}

func TestDrift_StatusStringFallback(t *testing.T) {
	assert.Equal(t, "unknown", DriftStatus(99).String())
}
