package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEmbedder_FakeDefault(t *testing.T) {
	for _, p := range []string{"", "fake", "FAKE"} {
		e, err := NewEmbedder(EmbedderConfig{Provider: p})
		require.NoError(t, err, p)
		require.NotNil(t, e, p)
		v, err := e(context.Background(), "hello world")
		require.NoError(t, err)
		assert.NotEmpty(t, v)
	}
}

func TestNewEmbedder_RequiresFields(t *testing.T) {
	cases := []struct {
		name string
		cfg  EmbedderConfig
		want string
	}{
		{"openai missing key", EmbedderConfig{Provider: "openai"}, "api key required"},
		{"openai-compat missing url", EmbedderConfig{Provider: "openai-compat", APIKey: "x", Model: "m"}, "base_url required"},
		{"openai-compat missing model", EmbedderConfig{Provider: "openai-compat", APIKey: "x", BaseURL: "https://x"}, "model required"},
		{"cohere missing key", EmbedderConfig{Provider: "cohere"}, "api key required"},
		{"ollama missing model", EmbedderConfig{Provider: "ollama"}, "model required"},
		{"localai missing model", EmbedderConfig{Provider: "localai"}, "model required"},
		{"unknown", EmbedderConfig{Provider: "qdrant"}, "unknown provider"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewEmbedder(tc.cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestNewEmbedder_RealConstructorsBuild(t *testing.T) {
	// 仅校验构造不报错——不真正发请求（chromem-go 工厂是惰性的）。
	cases := []EmbedderConfig{
		{Provider: "openai", APIKey: "sk-x"},
		{Provider: "openai", APIKey: "sk-x", Model: "text-embedding-3-large"},
		{Provider: "openai-compat", APIKey: "k", BaseURL: "https://x", Model: "m"},
		{Provider: "cohere", APIKey: "k"},
		{Provider: "ollama", Model: "nomic-embed-text"},
		{Provider: "ollama", Model: "nomic-embed-text", BaseURL: "http://host:11434/api"},
		{Provider: "localai", Model: "all-minilm"},
	}
	for _, cfg := range cases {
		e, err := NewEmbedder(cfg)
		require.NoError(t, err, cfg.Provider)
		require.NotNil(t, e, cfg.Provider)
	}
}
