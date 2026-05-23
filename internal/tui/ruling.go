package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
)

func formatTurnRuling(decision orchestrator.TurnDecision) string {
	lines := []string{}
	for _, mechanic := range decision.Mechanics {
		if line := formatMechanic(mechanic); line != "" {
			lines = append(lines, "- "+line)
		}
	}
	for _, check := range decision.Checks {
		if check.Passed || check.Code == "rules_applied" || check.Code == "tool_rejected" {
			continue
		}
		msg := strings.TrimSpace(check.Message)
		if msg == "" {
			msg = check.Code
		}
		lines = append(lines, "- 系统拦截："+msg)
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(append([]string{"裁定记录"}, lines...), "\n")
}

func formatMechanic(mechanic orchestrator.DecisionAction) string {
	if !mechanic.Success {
		detail := strings.TrimSpace(mechanic.Detail)
		if detail == "" {
			detail = mechanic.Tool
		}
		return "工具拒绝：" + detail
	}
	switch mechanic.Tool {
	case "roll_skill":
		var out struct {
			SkillName  string `json:"skill_name"`
			SkillValue int    `json:"skill_value"`
			Difficulty string `json:"difficulty"`
			Roll       int    `json:"roll"`
			Threshold  int    `json:"threshold"`
			Success    bool   `json:"success"`
			Degree     string `json:"degree"`
		}
		if err := json.Unmarshal([]byte(mechanic.Detail), &out); err != nil {
			return "技能检定：" + mechanic.Detail
		}
		return fmt.Sprintf("%s：%d → %d，%s（%s，阈值 %d）",
			nonEmpty(out.SkillName, "技能检定"),
			out.SkillValue,
			out.Roll,
			successLabel(out.Success),
			displayDifficulty(out.Difficulty),
			out.Threshold,
		)
	case "sanity_check":
		var out struct {
			Roll       int  `json:"roll"`
			Threshold  int  `json:"threshold"`
			Success    bool `json:"success"`
			Loss       int  `json:"loss"`
			NewSAN     int  `json:"new_san"`
			Indefinite bool `json:"triggered_indefinite_insanity"`
		}
		if err := json.Unmarshal([]byte(mechanic.Detail), &out); err != nil {
			return "SAN 检定：" + mechanic.Detail
		}
		line := fmt.Sprintf("SAN：%d → %d，%s，损失 %d，当前 SAN %d",
			out.Threshold, out.Roll, successLabel(out.Success), out.Loss, out.NewSAN)
		if out.Indefinite {
			line += "，触发不定性疯狂"
		}
		return line
	case "roll_damage":
		var out struct {
			Expression string `json:"expression"`
			Total      int    `json:"total"`
		}
		if err := json.Unmarshal([]byte(mechanic.Detail), &out); err != nil {
			return "伤害：" + mechanic.Detail
		}
		return fmt.Sprintf("伤害：%s = %d", nonEmpty(out.Expression, "骰子"), out.Total)
	case "opposed_roll":
		var out struct {
			Winner string `json:"winner"`
			Actor  struct {
				SkillName string `json:"skill_name"`
				Roll      int    `json:"roll"`
				Success   bool   `json:"success"`
			} `json:"actor"`
			Target struct {
				SkillName string `json:"skill_name"`
				Roll      int    `json:"roll"`
				Success   bool   `json:"success"`
			} `json:"target"`
		}
		if err := json.Unmarshal([]byte(mechanic.Detail), &out); err != nil {
			return "对抗检定：" + mechanic.Detail
		}
		return fmt.Sprintf("对抗：行动方 %d（%s） vs 对手 %d（%s），结果 %s",
			out.Actor.Roll, successLabel(out.Actor.Success),
			out.Target.Roll, successLabel(out.Target.Success),
			displayWinner(out.Winner),
		)
	default:
		return ""
	}
}

func successLabel(success bool) string {
	if success {
		return "成功"
	}
	return "失败"
}

func displayDifficulty(value string) string {
	switch value {
	case "hard":
		return "困难"
	case "extreme":
		return "极难"
	default:
		return "普通"
	}
}

func displayWinner(value string) string {
	switch value {
	case "actor":
		return "行动方胜"
	case "target":
		return "对手胜"
	case "tie":
		return "平手"
	default:
		return value
	}
}
