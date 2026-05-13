package i18n

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_LoadsBothLocales(t *testing.T) {
	for _, lang := range []string{"en", "zh-CN", ""} {
		tr, err := New(lang)
		require.NoError(t, err, lang)
		require.NotNil(t, tr)
		// 任一语言都应该能拿到 cli.smoke_passed
		got := tr.T("cli.smoke_passed")
		assert.NotContains(t, got, "missing", "T should resolve cli.smoke_passed under %q", lang)
		assert.NotContains(t, got, "i18n error")
	}
}

func TestNew_FallsBackOnUnknownLang(t *testing.T) {
	tr, err := New("xx-FAKE")
	require.NoError(t, err)
	got := tr.T("cli.smoke_passed")
	// 应该回退 zh-CN 的"冒烟检查通过"
	assert.Contains(t, got, "冒烟检查")
}

func TestT_RendersTemplateData(t *testing.T) {
	tr, err := New("en")
	require.NoError(t, err)
	got := tr.T("error.api_key_missing", map[string]any{"EnvVar": "ANTHROPIC_API_KEY"})
	assert.Contains(t, got, "ANTHROPIC_API_KEY")
	assert.Contains(t, got, "Set")

	gotZh, err := New("zh-CN")
	require.NoError(t, err)
	zh := gotZh.T("error.api_key_missing", map[string]any{"EnvVar": "ANTHROPIC_API_KEY"})
	assert.Contains(t, zh, "ANTHROPIC_API_KEY")
	assert.Contains(t, zh, "API key")
}

func TestT_MissingKeyReturnsBracketedHint(t *testing.T) {
	tr, err := New("en")
	require.NoError(t, err)
	got := tr.T("does.not.exist")
	assert.Equal(t, "[missing: does.not.exist]", got)
}

func TestDetectLang(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"empty all", map[string]string{}, "zh-CN"},
		{"LANG en_US.UTF-8", map[string]string{"LANG": "en_US.UTF-8"}, "en-US"},
		{"LANG zh_CN.UTF-8", map[string]string{"LANG": "zh_CN.UTF-8"}, "zh-CN"},
		{"LC_ALL wins over LANG", map[string]string{"LC_ALL": "en_US.UTF-8", "LANG": "zh_CN.UTF-8"}, "en-US"},
		{"C posix → fallback", map[string]string{"LANG": "C"}, "zh-CN"},
		{"with @modifier", map[string]string{"LANG": "de_DE.UTF-8@euro"}, "de-DE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LC_ALL", tc.env["LC_ALL"])
			t.Setenv("LC_MESSAGES", tc.env["LC_MESSAGES"])
			t.Setenv("LANG", tc.env["LANG"])
			assert.Equal(t, tc.want, DetectLang())
		})
	}
}

func TestNew_NilTranslatorIsSafe(t *testing.T) {
	var tr *Translator
	assert.Equal(t, "[i18n nil: foo]", tr.T("foo"))
	assert.Equal(t, "zh-CN", tr.Lang())
}
