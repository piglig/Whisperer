package orchestrator

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraceWriter_DisabledOnEmptyDir(t *testing.T) {
	for _, dir := range []string{"", "-"} {
		w := NewTraceWriter(dir, "save-1")
		assert.Empty(t, w.Path())
		assert.NoError(t, w.Append(TraceEntry{TurnNumber: 1}), dir)
	}
}

func TestTraceWriter_AppendsJSONL(t *testing.T) {
	dir := t.TempDir()
	w := NewTraceWriter(dir, "save-1")
	require.NotEmpty(t, w.Path())

	for i := 1; i <= 3; i++ {
		err := w.Append(TraceEntry{
			TurnNumber: i,
			UserInput:  "input " + string(rune('A'+i-1)),
		})
		require.NoError(t, err, "turn %d", i)
	}

	// 文件应在 <dir>/<save_id>/<ts>.jsonl
	assert.Contains(t, w.Path(), filepath.Join(dir, "save-1"))

	f, err := os.Open(w.Path())
	require.NoError(t, err)
	defer f.Close()

	var lines []map[string]any
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var m map[string]any
		require.NoError(t, json.Unmarshal(sc.Bytes(), &m), sc.Text())
		lines = append(lines, m)
	}
	require.NoError(t, sc.Err())

	require.Len(t, lines, 3)
	assert.Equal(t, "save-1", lines[0]["save_id"])
	assert.EqualValues(t, 1, lines[0]["turn_number"])
	assert.Equal(t, "input A", lines[0]["user_input"])
	assert.NotZero(t, lines[0]["trace_at_ms"])
	// 时间应严格递增
	assert.LessOrEqual(t, lines[0]["trace_at_ms"], lines[2]["trace_at_ms"])
}

func TestValidateTraceDir(t *testing.T) {
	assert.NoError(t, ValidateTraceDir(""))
	assert.NoError(t, ValidateTraceDir("-"))
	assert.NoError(t, ValidateTraceDir(t.TempDir()))
	assert.Error(t, ValidateTraceDir("~/somewhere"))
}

func TestTraceWriter_RedactsAPIKeys(t *testing.T) {
	dir := t.TempDir()
	w := NewTraceWriter(dir, "save-redact")
	require.NoError(t, w.Append(TraceEntry{
		TurnNumber: 1,
		UserInput:  "我把 sk-ant-api03-AbCdEfGhIjKlMnOpQrStUvWxYz0 给了酒馆老板",
		Result: TurnResult{
			Narrative: "GM: 你递出便条 sk-or-v1-XXXXXXXXXXXXXXXXXXXX 但她拒绝。",
		},
	}))

	raw, err := os.ReadFile(w.Path())
	require.NoError(t, err)
	body := string(raw)
	assert.NotContains(t, body, "sk-ant-api03-")
	assert.NotContains(t, body, "sk-or-v1-")
	assert.Contains(t, body, "***REDACTED***")
}
