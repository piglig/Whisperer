package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_MissingFileReturnsZeroConfig(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(filepath.Join(dir, "nope.toml"))
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "", cfg.Provider)
	assert.Equal(t, time.Duration(0), cfg.LLMTimeout)
}

func TestLoad_FullToml(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
provider = "openrouter"
model = "anthropic/claude-sonnet-4-7"
model_helper = "anthropic/claude-haiku-4-5"
scenario = "fog_harbor"
save = "abc"
variant = "rourke_runs"
seed = 42
db_path = "save.db"
memory_dir = "mem"
trace_dir = "runs"
meta_path = "runs/meta.json"
log_format = "json"
log_level = "debug"
llm_max_retries = 5
llm_timeout = "90s"

[embedder]
provider = "openai"
model = "text-embedding-3-large"
`), 0o644))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "openrouter", cfg.Provider)
	assert.Equal(t, "anthropic/claude-sonnet-4-7", cfg.Model)
	assert.Equal(t, int64(42), cfg.Seed)
	assert.Equal(t, 5, cfg.LLMMaxRetries)
	assert.Equal(t, 90*time.Second, cfg.LLMTimeout)
	assert.Equal(t, "openai", cfg.Embedder.Provider)
	assert.Equal(t, "text-embedding-3-large", cfg.Embedder.Model)
}

func TestLoad_RejectsBadDuration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	require.NoError(t, os.WriteFile(path, []byte(`llm_timeout = "120 deciseconds"`), 0o644))
	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "llm_timeout")
}

func TestLoad_TomlExplicitZeroRetries(t *testing.T) {
	// 显式 llm_max_retries = 0 应该保留为 0（关闭重试），不被默认值覆盖
	dir := t.TempDir()
	path := filepath.Join(dir, "zero.toml")
	require.NoError(t, os.WriteFile(path, []byte(`llm_max_retries = 0`), 0o644))
	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, 0, cfg.LLMMaxRetries)
	require.NotNil(t, cfg.LLMRetriesRaw)
	assert.Equal(t, 0, *cfg.LLMRetriesRaw)
}

func TestEnvOverlay_OverwritesEmptyAndPresentFields(t *testing.T) {
	cfg := &Config{Provider: "anthropic", Scenario: "fog_harbor"}
	t.Setenv("WHISPERER_PROVIDER", "openrouter")
	t.Setenv("WHISPERER_LOG_LEVEL", "warn")
	cfg.EnvOverlay()
	assert.Equal(t, "openrouter", cfg.Provider, "env should override toml")
	assert.Equal(t, "warn", cfg.LogLevel)
	assert.Equal(t, "fog_harbor", cfg.Scenario, "untouched fields stay")
}

func TestDefaultPath_HonorsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	assert.Equal(t, "/tmp/xdg/whisperer/config.toml", DefaultPath())
}
