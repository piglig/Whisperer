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
