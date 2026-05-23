package agent

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
)

//go:embed prompts/gm_system.tmpl
var gmSystemTmpl string

// PromptContext 是 GM system prompt 的渲染输入。所有字段都可空，模板会用 if 跳过。
type PromptContext struct {
	ScenarioContext   string
	InvestigatorBrief string
	LocationBrief     string
	RecentEvents      string
	// AvailableIDs 列出本剧本所有可被 tool 引用的实体 ID（locations / npcs / clues / items）。
	// 用于阻止 GM 自创不存在的 id（例如把 "harbor" 写成 "fog_harbor_docks"）。
	AvailableIDs string
	// Truth 是 GM-only 的本局真相（variant 决定）。模板会在专属段落中渲染并明确"不可剧透"。
	Truth string
	// NPCSecrets 是每位 NPC 在本局的隐藏动机表。GM 不可主动复述，但可影响 NPC 反应与场景描写。
	NPCSecrets string
	// NPCKnowledge 是每位 NPC 的关键词解锁知识表（玩家用相关词触碰时才"松口"）。
	NPCKnowledge string
	// ClueAtlas 是 tier 分级的线索网，让 GM 知道现在玩家在哪一层。
	ClueAtlas string
	// PlayerGuidance 是玩家可见的目标、地点行动提示与 NPC 初见素材。
	PlayerGuidance string
	// ThreatStatus 是当前局势风险轨状态，玩家可见，用于自然提醒倒计时和危险。
	ThreatStatus string
	// PlayerPrior 是跨周目 meta 渲染——玩家"已活过几次"、看过哪些 variant。GM 可用于"似曾相识"暗示。
	PlayerPrior string
}

// RenderGMSystem 把 PromptContext 渲染为完整 system prompt 字符串。
func RenderGMSystem(c PromptContext) (string, error) {
	t, err := template.New("gm_system").Parse(gmSystemTmpl)
	if err != nil {
		return "", fmt.Errorf("parse gm_system template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, c); err != nil {
		return "", fmt.Errorf("render gm_system template: %w", err)
	}
	return buf.String(), nil
}
