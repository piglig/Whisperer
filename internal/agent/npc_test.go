package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNPCAgent_Speak_Basic(t *testing.T) {
	llm := &fakeLLM{
		scripts: []string{msgWith(textBlk("我不知道你在说什么。"))},
	}
	a := NewNPC(llm, "")
	out, err := a.Speak(context.Background(), NPCSpeakRequest{
		Persona:    "Name: Vance\nPersonality: stern doctor",
		Intent:     "回应玩家关于黑暗仪式的询问，态度警惕",
		PlayerLine: "你听说过那场黑暗仪式吗？",
	})
	require.NoError(t, err)
	assert.Equal(t, "我不知道你在说什么。", out)
}

func TestNPCAgent_Speak_NoPlayerLine(t *testing.T) {
	llm := &fakeLLM{scripts: []string{msgWith(textBlk("「夜雾来了。」"))}}
	a := NewNPC(llm, ModelHelper)
	out, err := a.Speak(context.Background(), NPCSpeakRequest{
		Persona: "Name: Vance",
		Intent:  "主动开口提到雾气",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, out)
}

func TestNPCAgent_RequiresPersona(t *testing.T) {
	a := NewNPC(&fakeLLM{}, "")
	_, err := a.Speak(context.Background(), NPCSpeakRequest{Intent: "x"})
	assert.ErrorContains(t, err, "Persona is required")
}

func TestNPCAgent_NoLLM(t *testing.T) {
	a := &NPCAgent{}
	_, err := a.Speak(context.Background(), NPCSpeakRequest{Persona: "x"})
	assert.ErrorContains(t, err, "llm is nil")
}

func TestNPCAgent_EmptyResponse(t *testing.T) {
	llm := &fakeLLM{scripts: []string{msgWith(textBlk(""))}}
	a := NewNPC(llm, "")
	_, err := a.Speak(context.Background(), NPCSpeakRequest{Persona: "x", Intent: "y"})
	assert.ErrorContains(t, err, "empty")
}
