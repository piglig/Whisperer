package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSlash_Empty(t *testing.T) {
	_, ok := parseSlash("")
	assert.False(t, ok)
	_, ok = parseSlash("/")
	assert.False(t, ok)
	_, ok = parseSlash("hello")
	assert.False(t, ok)
}

func TestParseSlash_NoArg(t *testing.T) {
	c, ok := parseSlash("/quit")
	assert.True(t, ok)
	assert.Equal(t, "quit", c.name)
	assert.Empty(t, c.arg)
}

func TestParseSlash_GenericArg(t *testing.T) {
	c, ok := parseSlash("/bind lyra")
	assert.True(t, ok)
	assert.Equal(t, "bind", c.name)
	assert.Equal(t, "lyra", c.arg)
	assert.Equal(t, "lyra", c.rest)
}

func TestParseSlash_TalkSplitsNPCAndRest(t *testing.T) {
	c, ok := parseSlash("/talk vance 你看到什么了？")
	assert.True(t, ok)
	assert.Equal(t, "talk", c.name)
	assert.Equal(t, "vance", c.arg)
	assert.Equal(t, "你看到什么了？", c.rest)
}

func TestParseSlash_TalkOnlyNPC(t *testing.T) {
	c, ok := parseSlash("/talk vance")
	assert.True(t, ok)
	assert.Equal(t, "vance", c.arg)
	assert.Empty(t, c.rest)
}

func TestParseSlash_LowercaseName(t *testing.T) {
	c, ok := parseSlash("/QUIT")
	assert.True(t, ok)
	assert.Equal(t, "quit", c.name)
}
