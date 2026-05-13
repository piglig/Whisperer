package secrets

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedact_KnownPatterns(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"anthropic", "key=sk-ant-api03-AbCdEfGhIjKlMnOpQrStUvWxYz_-2345 next"},
		{"openrouter", "OPENROUTER_API_KEY=sk-or-v1-AbCdEfGhIjKlMnOp1234567890ABcd done"},
		{"openai-proj", "sk-proj-ZZZZAaaaBBBB99887766554433221100 yes"},
		{"bearer", "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.zzzAAAbbbCCCdddEEE000"},
		{"plain-jwt", "token: eyJabc123XYZdefGHI789.eyJpYXQiOjE2NDg3MTM2MDB9.signaturevalueparthere000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := Redact(tc.input)
			assert.NotContains(t, out, "sk-ant-api03-")
			assert.NotContains(t, out, "sk-or-v1-")
			assert.Contains(t, out, "***REDACTED***")
		})
	}
}

func TestRedact_DoesNotMangleNormalText(t *testing.T) {
	innocuous := []string{
		"我去酒馆找玛丽莎",
		"call vance and ask about the ledger",
		"短的 hex abc123",
		"",
		"sk-ant-",      // 太短，不命中
		"Bearer short", // 不够长
	}
	for _, s := range innocuous {
		assert.Equal(t, s, Redact(s), "should pass through: %q", s)
	}
}

func TestRedact_MultipleKeysInOneString(t *testing.T) {
	in := `set both: sk-ant-api03-FFFFFFFFFFFFFFFFFFFF and sk-or-v1-AAAAAAAAAAAAAAAAAAAA done`
	out := Redact(in)
	assert.NotContains(t, out, "sk-ant-api03-")
	assert.NotContains(t, out, "sk-or-v1-")
	// 两条独立 key 都应被替换。
	assert.Equal(t, 2, strings.Count(out, "***REDACTED***"))
}
