package log

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_RedactsSensitive(t *testing.T) {
	var buf bytes.Buffer
	l := New("json", "debug", &buf)
	l.Info("auth check",
		"api_key", "sk-ant-real-secret-here",
		"auth_token", "Bearer XYZ",
		"secret_field", "do not leak",
		"safe_field", "ok-to-show",
	)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))

	assert.Equal(t, "***", entry["api_key"])
	assert.Equal(t, "***", entry["auth_token"])
	assert.Equal(t, "***", entry["secret_field"])
	assert.Equal(t, "ok-to-show", entry["safe_field"])
	// 内置字段不应被屏蔽
	assert.Equal(t, "auth check", entry["msg"])
	assert.Equal(t, "INFO", entry["level"])
}

func TestNew_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	l := New("text", "info", &buf)
	l.Warn("hello world", "foo", "bar")

	out := buf.String()
	assert.Contains(t, out, "level=WARN")
	assert.Contains(t, out, `msg="hello world"`)
	assert.Contains(t, out, "foo=bar")
}

func TestNew_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := New("text", "warn", &buf)
	l.Debug("noisy")
	l.Info("info-level")
	l.Warn("important")
	l.Error("bad")

	out := buf.String()
	assert.NotContains(t, out, "noisy")
	assert.NotContains(t, out, "info-level")
	assert.Contains(t, out, "important")
	assert.Contains(t, out, "bad")
}

func TestNew_UnknownLevelFallsBackToInfo(t *testing.T) {
	var buf bytes.Buffer
	l := New("text", "garbage", &buf)
	l.Debug("d")
	l.Info("i")
	out := buf.String()
	assert.NotContains(t, out, " d ")
	assert.True(t, strings.Contains(out, "msg=i"))
}
