package tools

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/store"
)

func TestGetTimeOfDay_Default(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	out, isErr := d.Dispatch(ctx, "get_time_of_day", json.RawMessage(`{}`))
	require.False(t, isErr)
	b, _ := json.Marshal(out)
	assert.Contains(t, string(b), `"morning"`)
}

func TestAdvanceTime_DefaultOneStage(t *testing.T) {
	s, d, saveID, ctx := newTestEnv(t)
	_, isErr := d.Dispatch(ctx, "advance_time", json.RawMessage(`{}`))
	require.False(t, isErr)
	sv, _ := s.Repo().GetSave(ctx, saveID)
	assert.Equal(t, store.TimeAfternoon, sv.TimeOfDay)
}

func TestAdvanceTime_MultipleStages(t *testing.T) {
	s, d, saveID, ctx := newTestEnv(t)
	_, isErr := d.Dispatch(ctx, "advance_time", json.RawMessage(`{"stages":3}`))
	require.False(t, isErr)
	sv, _ := s.Repo().GetSave(ctx, saveID)
	assert.Equal(t, store.TimeMorning, sv.TimeOfDay) // morning + 3 = morning
}

func TestAdvanceTime_BadJSON(t *testing.T) {
	_, d, _, ctx := newTestEnv(t)
	_, isErr := d.Dispatch(ctx, "advance_time", json.RawMessage(`{`))
	assert.True(t, isErr)
}

func TestGetTimeOfDay_MissingSave(t *testing.T) {
	_, d, saveID, ctx := newTestEnv(t)
	require.NoError(t, d.repo.DeleteSave(ctx, saveID))
	_, isErr := d.Dispatch(ctx, "get_time_of_day", json.RawMessage(`{}`))
	assert.True(t, isErr)
}
