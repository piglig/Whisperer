package orchestrator

import "strings"

func buildActionBrief(action PlayerAction, guard ActionGuardResult) string {
	lines := []string{
		"本回合玩家行动：",
		"- 类型：" + actionKindChinese(action.Kind),
	}
	if action.Text != "" {
		lines = append(lines, "- 文本："+action.Text)
	}
	if len(action.Targets) > 0 {
		targets := make([]string, 0, len(action.Targets))
		for _, target := range action.Targets {
			label := target.Kind + " " + target.ID
			if target.Name != "" {
				label += "（" + target.Name + "）"
			}
			targets = append(targets, label)
		}
		lines = append(lines, "- 目标："+strings.Join(targets, "；"))
	}
	if action.Source.Kind != "" {
		lines = append(lines, "- 来源："+action.Source.Kind+" "+action.Source.ID)
	}
	if guard.Allowed {
		lines = append(lines, "- 规则门卫：已允许")
	} else {
		lines = append(lines, "- 规则门卫：已拒绝："+guard.Reason)
	}
	lines = append(lines,
		"",
		"裁定边界：",
		"- 本回合只围绕上述行动和目标裁定。",
		"- 不要把一次行动扩展成自动访问其他地点、自动盘问其他 NPC、自动发现未触发的关键线索。",
		"- 若需要状态变化，必须先调用对应 tool；tool 的结果高于叙事。",
	)
	return strings.Join(lines, "\n")
}

func buildGMUserInput(action PlayerAction, guard ActionGuardResult) string {
	text := strings.TrimSpace(action.Text)
	if text == "" {
		text = strings.TrimSpace(action.Raw)
	}
	brief := buildActionBrief(action, guard)
	if text == "" {
		return brief
	}
	return text + "\n\n" + brief
}

func actionKindChinese(kind TurnIntent) string {
	switch kind {
	case IntentInvestigate:
		return "调查"
	case IntentTalk:
		return "交谈"
	case IntentMove:
		return "移动"
	case IntentUseItem:
		return "使用物品"
	case IntentConfront:
		return "对峙"
	case IntentReport:
		return "结案"
	default:
		return "未知"
	}
}
