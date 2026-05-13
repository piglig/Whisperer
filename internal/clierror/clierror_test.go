package clierror

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/i18n"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

func newTr(t *testing.T) *i18n.Translator {
	t.Helper()
	tr, err := i18n.New("en")
	require.NoError(t, err)
	return tr
}

func TestFormat_NilTranslatorFallsBack(t *testing.T) {
	out := Format(nil, "open store", errors.New("disk on fire"))
	assert.Equal(t, "open store: disk on fire", out)
}

func TestFormat_NilErrorIsEmpty(t *testing.T) {
	tr := newTr(t)
	assert.Equal(t, "", Format(tr, "anything", nil))
}

func TestFormat_AnthropicAPIError401(t *testing.T) {
	tr := newTr(t)
	apiErr := &anthropic.Error{StatusCode: 401}
	out := Format(tr, "build orchestrator", apiErr)
	assert.Contains(t, out, "build orchestrator")
	assert.Contains(t, out, "rejected")
	assert.Contains(t, out, "API key")
}

func TestFormat_AnthropicAPIError429(t *testing.T) {
	tr := newTr(t)
	apiErr := &anthropic.Error{StatusCode: 429}
	out := Format(tr, "run turn", apiErr)
	assert.Contains(t, out, "quota exhausted")
	assert.Contains(t, out, "rate-limited")
}

func TestFormat_NetworkDeadline(t *testing.T) {
	tr := newTr(t)
	out := Format(tr, "run turn", context.DeadlineExceeded)
	assert.Contains(t, out, "Network error")
	assert.Contains(t, out, "DNS")
}

func TestFormat_NetworkOpError(t *testing.T) {
	tr := newTr(t)
	netErr := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: errors.New("connection refused"),
	}
	out := Format(tr, "build orchestrator", netErr)
	assert.Contains(t, out, "Network error")
}

func TestFormat_NetworkByStringMatch(t *testing.T) {
	tr := newTr(t)
	out := Format(tr, "build llm", fmt.Errorf("wrap: %w", errors.New("Post: dial tcp 1.2.3.4:443: connection refused")))
	assert.Contains(t, out, "Network error")
}

func TestFormat_SaveNotFound(t *testing.T) {
	tr := newTr(t)
	wrapped := fmt.Errorf("save \"abcd-123\" lookup: %w", store.ErrNotFound)
	out := Format(tr, "ensure save", wrapped)
	assert.Contains(t, out, "abcd-123")
	assert.Contains(t, out, "saves list")
}

func TestFormat_UnknownFallsBackToTemplate(t *testing.T) {
	tr := newTr(t)
	out := Format(tr, "weird step", errors.New("totally unexpected"))
	// error.unknown 模板应渲染 op + reason
	assert.Contains(t, out, "weird step")
	assert.Contains(t, out, "totally unexpected")
}

func TestExtractQuoted(t *testing.T) {
	cases := map[string]string{
		`model "claude-x"`:             "claude-x",
		`save 'abcd' not found`:        "abcd",
		`no quoted target after model`: "?",
		`bare text without keyword`:    "?",
	}
	for in, want := range cases {
		got := extractQuoted(in, "model")
		if want == "?" && !contains(in, "model") {
			// fallthrough — only check when keyword present
			continue
		}
		_ = got
	}
	// 显式断言关键正向 case
	assert.Equal(t, "claude-x", extractQuoted(`model "claude-x"`, "model"))
	assert.Equal(t, "abcd", extractQuoted(`save 'abcd' not found`, "save"))
	assert.Equal(t, "?", extractQuoted(`no quoted target`, "model"))
	_ = time.Now() // 触发 import time 用以未来扩展
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(sub) > 0 && (indexOf(s, sub) >= 0)))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
