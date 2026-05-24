package tui

import (
	"fmt"
	"strings"

	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

func (m *Model) appendTurnResult(res orchestrator.TurnResult) {
	if res.Narrative != "" {
		m.log = append(m.log, logEntry{kind: EntryGM, text: res.Narrative})
	}
	if review := formatTurnReview(res); review != "" {
		m.log = append(m.log, logEntry{kind: EntryReview, text: review})
	}
	if ruling := formatTurnRuling(res.Decision); ruling != "" {
		m.log = append(m.log, logEntry{kind: EntrySystem, text: ruling})
	}
	if !res.Summary.Empty() {
		m.log = append(m.log, logEntry{kind: EntrySystem, text: formatTurnSummary(res.Summary)})
	}
	for range res.Fired {
		m.log = append(m.log, logEntry{
			kind: EntrySystem,
			text: "故事状态已更新：新的线索或局势变化已记录。",
		})
	}
	switch res.Drift.String() {
	case "soft":
		m.drift = "软偏离：建议关注主线"
	case "hard":
		m.drift = "硬偏离：剧本进入失败收束"
	default:
		m.drift = ""
	}
	if !res.SLAReport.Passed && len(res.SLAReport.Violations) > 0 {
		m.log = append(m.log, logEntry{
			kind: EntryError,
			text: "刚才的叙事和游戏状态可能不完全一致；系统已保留当前可用结果。",
		})
	}
	if res.Ending != nil && !m.endingShown {
		m.endingShown = true
		text := fmt.Sprintf("[%s] %s", strings.ToUpper(res.Ending.Kind), res.Ending.Description)
		if res.Report != nil {
			text = formatCaseReport(res.Report)
		}
		m.log = append(m.log, logEntry{
			kind: EntryEnding,
			text: text,
		})
	}
	m.storyOffset = 0
}

func formatTurnReview(res orchestrator.TurnResult) string {
	lines := []string{"回合复盘"}
	action := strings.TrimSpace(res.Decision.Action.Text)
	if action == "" {
		action = strings.TrimSpace(res.Action.Text)
	}
	if action == "" {
		action = strings.TrimSpace(res.Decision.PlayerInput)
	}
	if action != "" {
		lines = append(lines, "- 行动："+actionKindLabel(res.Decision.Intent)+" · "+action)
	}
	if !res.Summary.Empty() {
		lines = append(lines, reviewSummaryLines(res.Summary)...)
	}
	for _, check := range res.Decision.Checks {
		if check.Passed || strings.TrimSpace(check.Message) == "" {
			continue
		}
		lines = append(lines, "- 未执行："+check.Message)
	}
	if len(res.Decision.Mechanics) > 0 {
		count := 0
		for _, mechanic := range res.Decision.Mechanics {
			if line := formatMechanic(mechanic); line != "" {
				lines = append(lines, "- 裁定："+line)
				count++
			}
			if count >= 2 {
				break
			}
		}
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func reviewSummaryLines(summary orchestrator.TurnSummary) []string {
	lines := []string{}
	if summary.LocationChange != nil {
		lines = append(lines, fmt.Sprintf("- 位置：%s → %s", nonEmpty(summary.LocationChange.From, "?"), nonEmpty(summary.LocationChange.To, "?")))
	}
	if summary.TimeChange != nil {
		lines = append(lines, fmt.Sprintf("- 时间：%s → %s", displayTimeOfDay(store.TimeOfDay(summary.TimeChange.From)), displayTimeOfDay(store.TimeOfDay(summary.TimeChange.To))))
	}
	if summary.StageChange != nil {
		lines = append(lines, fmt.Sprintf("- 阶段：%s → %s", displayStage(summary.StageChange.From), displayStage(summary.StageChange.To)))
	}
	for _, clue := range summary.NewClues {
		lines = append(lines, "- 新线索："+nonEmpty(clue.Description, clue.ID))
	}
	for _, npc := range summary.NPCChanges {
		if npc.AliveChanged {
			state := "存活"
			if !npc.AliveTo {
				state = "死亡"
			}
			lines = append(lines, fmt.Sprintf("- 人物：%s 状态变为%s", nonEmpty(npc.Name, npc.ID), state))
		}
		if npc.RelationFrom != npc.RelationTo {
			lines = append(lines, fmt.Sprintf("- 关系：%s %+d → %+d", nonEmpty(npc.Name, npc.ID), npc.RelationFrom, npc.RelationTo))
		}
	}
	for _, threat := range summary.ThreatChanges {
		lines = append(lines, fmt.Sprintf("- 风险：%s %s → %s", threat.Name, threat.From, threat.To))
	}
	return lines
}

func formatTurnSummary(summary orchestrator.TurnSummary) string {
	lines := []string{"回合摘要"}
	if summary.LocationChange != nil {
		lines = append(lines, fmt.Sprintf("- 位置：%s → %s", nonEmpty(summary.LocationChange.From, "?"), nonEmpty(summary.LocationChange.To, "?")))
	}
	if summary.TimeChange != nil {
		lines = append(lines, fmt.Sprintf("- 时间：%s → %s", displayTimeOfDay(store.TimeOfDay(summary.TimeChange.From)), displayTimeOfDay(store.TimeOfDay(summary.TimeChange.To))))
	}
	if summary.StageChange != nil {
		lines = append(lines, fmt.Sprintf("- 阶段：%s → %s", displayStage(summary.StageChange.From), displayStage(summary.StageChange.To)))
	}
	for _, clue := range summary.NewClues {
		lines = append(lines, "- 新线索："+nonEmpty(clue.Description, clue.ID))
	}
	for _, npc := range summary.NPCChanges {
		if npc.AliveChanged {
			state := "存活"
			if !npc.AliveTo {
				state = "死亡"
			}
			lines = append(lines, fmt.Sprintf("- 人物：%s 状态变为%s", nonEmpty(npc.Name, npc.ID), state))
		}
		if npc.RelationFrom != npc.RelationTo {
			lines = append(lines, fmt.Sprintf("- 关系：%s %+d → %+d", nonEmpty(npc.Name, npc.ID), npc.RelationFrom, npc.RelationTo))
		}
	}
	for _, threat := range summary.ThreatChanges {
		lines = append(lines, fmt.Sprintf("- 风险：%s %s → %s", threat.Name, threat.From, threat.To))
	}
	return strings.Join(lines, "\n")
}

func formatCaseReport(report *scenario.CaseReport) string {
	if report == nil {
		return ""
	}
	lines := []string{
		fmt.Sprintf("[%s] %s", strings.ToUpper(report.Ending.Kind), report.Ending.Description),
	}
	if report.EvidenceStatus != "" {
		lines = append(lines, "", "结案评估", "  "+report.EvidenceStatus)
	}
	if report.CulpritName != "" {
		lines = append(lines, "", "本局真凶", "  "+report.CulpritName)
	}
	if len(report.FoundKeyClues) > 0 {
		lines = append(lines, "", "已掌握关键证据")
		for _, clue := range report.FoundKeyClues {
			lines = append(lines, "  - "+clue.Description)
		}
	}
	if len(report.MissingKeyClues) > 0 {
		lines = append(lines, "", "遗漏关键证据")
		for _, clue := range report.MissingKeyClues {
			lines = append(lines, "  - "+nonEmpty(clue.Description, clue.ID))
			for _, lead := range clue.Leads {
				lines = append(lines, "      下次试试 · "+lead.Hint)
			}
		}
	}
	if npcLines := reportNPCOutcomeLines(report.NPCOutcomes); len(npcLines) > 0 {
		lines = append(lines, "", "人物后果")
		lines = append(lines, npcLines...)
	}
	if threatLines := reportThreatLines(report.Threats); len(threatLines) > 0 {
		lines = append(lines, "", "风险结算")
		lines = append(lines, threatLines...)
	}
	if report.TruthSummary != "" {
		lines = append(lines, "", "本局真相", "  "+report.TruthSummary)
	}
	return strings.Join(lines, "\n")
}

func reportNPCOutcomeLines(outcomes []scenario.NPCOutcome) []string {
	lines := []string{}
	for _, outcome := range outcomes {
		if outcome.Alive && outcome.Relation > -15 {
			continue
		}
		state := "存活"
		if !outcome.Alive {
			state = "死亡"
		}
		lines = append(lines, fmt.Sprintf("  - %s：%s，关系 %+d", nonEmpty(outcome.Name, outcome.ID), state, outcome.Relation))
	}
	return lines
}

func reportThreatLines(threats []scenario.ThreatStatus) []string {
	lines := []string{}
	for _, threat := range threats {
		if threat.Severity == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("  - %s：%s", threat.Name, threat.StateLabel))
	}
	return lines
}
