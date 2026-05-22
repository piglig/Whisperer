package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/i18n"
)

func newTestEnv(t *testing.T, input string) (*wizardEnv, *bytes.Buffer) {
	t.Helper()
	tr, err := i18n.New("en")
	require.NoError(t, err)

	out := &bytes.Buffer{}
	return &wizardEnv{
		in:  bufio.NewReader(strings.NewReader(input)),
		out: out,
		tr:  tr,
	}, out
}

func TestRunWizard_AnthropicHappyPath(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	// 输入：anthropic / sk-test / fog_harbor / auto / y
	env, out := newTestEnv(t, "anthropic\nsk-ant-xxx\nfog_harbor\nauto\ny\n")

	written, result, err := runWizard(env, configPath)
	require.NoError(t, err)
	assert.True(t, written)
	assert.Equal(t, "anthropic", result.Provider)
	assert.Equal(t, "ANTHROPIC_API_KEY", result.APIKeyEnv)
	assert.Equal(t, "fog_harbor", result.Scenario)
	assert.Empty(t, result.Lang)

	body, err := os.ReadFile(configPath)
	require.NoError(t, err)
	bodyStr := string(body)
	assert.Contains(t, bodyStr, `provider = "anthropic"`)
	assert.Contains(t, bodyStr, `scenario = "fog_harbor"`)
	// API key 一定不写盘
	assert.NotContains(t, bodyStr, "sk-ant-xxx")
	assert.NotContains(t, bodyStr, "api_key")

	// 用户可见输出含友好欢迎与确认
	outStr := out.String()
	assert.Contains(t, outStr, "first-run setup")
	assert.Contains(t, outStr, "✓")
}

func TestRunWizard_LangPersistedToConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	env, _ := newTestEnv(t, "anthropic\nsk\nfog_harbor\nzh-CN\ny\n")
	written, result, err := runWizard(env, configPath)
	require.NoError(t, err)
	assert.True(t, written)
	assert.Equal(t, "zh-CN", result.Lang)

	body, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(body), `lang = "zh-CN"`)
}

func TestRunWizard_AbortOnConfirmN(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	env, out := newTestEnv(t, "openrouter\nsk-or-xxx\nfog_harbor\nzh-CN\nn\n")
	written, result, err := runWizard(env, configPath)
	require.NoError(t, err)
	assert.False(t, written)
	assert.Equal(t, "openrouter", result.Provider)
	assert.Equal(t, "OPENROUTER_API_KEY", result.APIKeyEnv)

	_, statErr := os.Stat(configPath)
	assert.True(t, os.IsNotExist(statErr), "file should not exist when aborted")

	assert.Contains(t, out.String(), "Aborted")
}

func TestRunWizard_BlankAPIKeyWarns(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	// 留空 API key
	env, out := newTestEnv(t, "anthropic\n\nfog_harbor\nauto\ny\n")
	written, result, err := runWizard(env, configPath)
	require.NoError(t, err)
	assert.True(t, written)
	assert.Empty(t, result.APIKey)
	assert.Contains(t, out.String(), "ANTHROPIC_API_KEY")
	assert.Contains(t, out.String(), "Hint")
}

func TestRunWizard_RetriesOnInvalidProvider(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	// 第一次 garbage，第二次 anthropic
	env, out := newTestEnv(t, "garbage\nanthropic\nsk\nfog_harbor\nauto\ny\n")
	written, result, err := runWizard(env, configPath)
	require.NoError(t, err)
	assert.True(t, written)
	assert.Equal(t, "anthropic", result.Provider)
	assert.Contains(t, out.String(), "Invalid choice")
}

func TestRunWizard_OpenAIProvider(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	env, _ := newTestEnv(t, "openai\nsk-test\nfog_harbor\nauto\ny\n")
	written, result, err := runWizard(env, configPath)
	require.NoError(t, err)
	assert.True(t, written)
	assert.Equal(t, "openai", result.Provider)
	assert.Equal(t, "OPENAI_API_KEY", result.APIKeyEnv)
}

func TestRunWizard_NumberedChoices(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	env, out := newTestEnv(t, "3\nsk-test\n1\n2\ny\n")
	written, result, err := runWizard(env, configPath)
	require.NoError(t, err)
	assert.True(t, written)
	assert.Equal(t, "grok", result.Provider)
	assert.Equal(t, "XAI_API_KEY", result.APIKeyEnv)
	assert.Equal(t, "fog_harbor", result.Scenario)
	assert.Equal(t, "zh-CN", result.Lang)

	outStr := out.String()
	assert.Contains(t, outStr, "1. anthropic")
	assert.Contains(t, outStr, "2. openai")
	assert.Contains(t, outStr, "雾港疑案")
	assert.Contains(t, outStr, "1. 自动检测")
}

func TestShouldRunWizard(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "exists.toml")
	require.NoError(t, os.WriteFile(existing, []byte("provider = \"anthropic\""), 0o644))

	// init 子命令永远跑（即使配置已存在）
	assert.True(t, shouldRunWizard([]string{"init"}, existing))

	// 文件存在 + 没显式 init → 不跑
	assert.False(t, shouldRunWizard([]string{}, existing))

	// 文件缺失分支留给 stdinIsTTY 判定，是环境相关的——在不同 shell / WSL /
	// CI 容器里都不一致；不在单测里覆盖该分支。
}
