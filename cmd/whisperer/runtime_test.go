package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMemory_Off(t *testing.T) {
	mem, mode, err := buildMemory(memoryRuntimeConfig{Kind: "off"})
	require.NoError(t, err)
	assert.Nil(t, mem)
	assert.Equal(t, "off", mode)
}

func TestBuildMemory_Fake(t *testing.T) {
	mem, mode, err := buildMemory(memoryRuntimeConfig{Kind: "fake"})
	require.NoError(t, err)
	require.NotNil(t, mem)
	assert.Equal(t, "fake", mode)
	assert.NoError(t, mem.Close())
}

func TestBuildMemory_OpenAIRequiresKey(t *testing.T) {
	t.Setenv("WHISPERER_TEST_EMBED_KEY", "")
	_, _, err := buildMemory(memoryRuntimeConfig{
		Kind:   "openai",
		KeyEnv: "WHISPERER_TEST_EMBED_KEY",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WHISPERER_TEST_EMBED_KEY")
}

func TestBuildMemory_OpenAIUsesExplicitKey(t *testing.T) {
	t.Setenv("WHISPERER_TEST_EMBED_KEY", "")
	mem, mode, err := buildMemory(memoryRuntimeConfig{
		Kind:   "openai",
		Key:    "sk-test",
		KeyEnv: "WHISPERER_TEST_EMBED_KEY",
	})
	require.NoError(t, err)
	require.NotNil(t, mem)
	assert.Equal(t, "openai", mode)
	assert.NoError(t, mem.Close())
}

func TestBuildMemory_UnknownKind(t *testing.T) {
	_, _, err := buildMemory(memoryRuntimeConfig{Kind: "bogus"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown embedder")
}
