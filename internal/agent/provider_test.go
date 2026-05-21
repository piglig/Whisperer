package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseProvider(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantEnv string
	}{
		{"", ProviderAnthropic, "ANTHROPIC_API_KEY"},
		{"anthropic", ProviderAnthropic, "ANTHROPIC_API_KEY"},
		{"or", ProviderOpenRouter, "OPENROUTER_API_KEY"},
		{"openai", ProviderOpenAI, "OPENAI_API_KEY"},
		{"xai", ProviderGrok, "XAI_API_KEY"},
		{"google", ProviderGemini, "GEMINI_API_KEY"},
	}
	for _, tc := range cases {
		got, ok := ParseProvider(tc.in)
		require.True(t, ok, tc.in)
		assert.Equal(t, tc.want, got.Name)
		assert.Equal(t, tc.wantEnv, got.EnvKey)
	}

	_, ok := ParseProvider("unknown")
	assert.False(t, ok)
	assert.Equal(t, ProviderAnthropic, NormalizeProvider("unknown"))
}

func TestAutoProvider(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("XAI_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	assert.Equal(t, ProviderAnthropic, AutoProvider())

	t.Setenv("OPENAI_API_KEY", "sk-test")
	assert.Equal(t, ProviderOpenAI, AutoProvider())

	t.Setenv("XAI_API_KEY", "xai-test")
	assert.Equal(t, ProviderAnthropic, AutoProvider(), "multiple provider keys fall back to explicit default")
}

func TestBuildLLM(t *testing.T) {
	llm, gm, helper := BuildLLM(ProviderOpenAI, "sk-test", 1, time.Second)
	assert.IsType(t, &OpenAIChat{}, llm)
	assert.Equal(t, OpenAIModelGM, gm)
	assert.Equal(t, OpenAIModelHelper, helper)

	llm, gm, helper = BuildLLM(ProviderAnthropic, "sk-test", 1, time.Second)
	assert.IsType(t, &Anthropic{}, llm)
	assert.Equal(t, ModelGM, gm)
	assert.Equal(t, ModelHelper, helper)
}
