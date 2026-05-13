package agent

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/anthropics/anthropic-sdk-go"
)

//go:embed prompts/npc_system.tmpl
var npcSystemTmpl string

// NPCSpeakRequest 是 NPCAgent.Speak 的输入。所有字段都可空（除了 PlayerLine 在 Intent
// 缺省时至少应有一个，否则 NPC 没有可回应的对象）。
type NPCSpeakRequest struct {
	Persona       string // NPC 人格档案
	Secret        string // 本 NPC 的隐藏动机 / 私下立场，仅本子代理可见
	Knowledge     string // 关键词解锁的隐藏知识表（NPC 自行判断玩家本回合输入是否击中）
	RecentHistory string // memory 检索得到的近期事件摘要
	Intent        string // GM 给出的本次台词意图（如 "回应玩家关于黑暗仪式的询问，态度警惕"）
	PlayerLine    string // 玩家本次说的话；可空
}

// NPCAgent 是只生成单段对白的轻量 agent。无工具调用。
type NPCAgent struct {
	llm       LLM
	model     anthropic.Model
	maxTokens int64
}

// NewNPC 构造一个 NPCAgent。model 留空时默认 Haiku。
func NewNPC(llm LLM, model anthropic.Model) *NPCAgent {
	if model == "" {
		model = ModelHelper
	}
	return &NPCAgent{llm: llm, model: model, maxTokens: 512}
}

// Speak 调用 LLM 生成一段 NPC 台词。返回拼接后的纯文本。
func (n *NPCAgent) Speak(ctx context.Context, req NPCSpeakRequest) (string, error) {
	if n.llm == nil {
		return "", errors.New("NPCAgent: llm is nil")
	}
	if strings.TrimSpace(req.Persona) == "" {
		return "", errors.New("NPCAgent.Speak: Persona is required")
	}

	system, err := renderNPCSystem(req)
	if err != nil {
		return "", err
	}

	userText := req.PlayerLine
	if strings.TrimSpace(userText) == "" {
		// 没有玩家直接问句时，用 GM 意图作为提示，要求 NPC 主动开口。
		userText = "(请按 GM 指示开口说话)"
	}

	msg, err := n.llm.NewMessage(ctx, anthropic.MessageNewParams{
		Model:     n.model,
		MaxTokens: n.maxTokens,
		System: []anthropic.TextBlockParam{
			{Text: system, CacheControl: anthropic.NewCacheControlEphemeralParam()},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userText)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("NPCAgent: LLM call: %w", err)
	}

	var b strings.Builder
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(t.Text)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", errors.New("NPCAgent: empty response")
	}
	return out, nil
}

func renderNPCSystem(req NPCSpeakRequest) (string, error) {
	t, err := template.New("npc_system").Parse(npcSystemTmpl)
	if err != nil {
		return "", fmt.Errorf("parse npc_system template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, req); err != nil {
		return "", fmt.Errorf("render npc_system template: %w", err)
	}
	return buf.String(), nil
}
