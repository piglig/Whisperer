package director

import (
	"sort"
	"strings"

	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

type Urgency string

const (
	UrgencyNormal   Urgency = "normal"
	UrgencyWarning  Urgency = "warning"
	UrgencyCritical Urgency = "critical"
)

type Snapshot struct {
	ScenarioID     string
	Stage          string
	Turn           int
	LocationID     string
	TimeOfDay      store.TimeOfDay
	FoundClues     map[string]bool
	FiredTriggers  map[string]bool
	Threats        []scenario.ThreatStatus
	AvailableNPCs  []store.NPC
	AvailableItems []store.Item
	Objective      scenario.Objective
}

type Advice struct {
	PrimaryObjective string   `json:"primary_objective"`
	Reason           string   `json:"reason"`
	Urgency          Urgency  `json:"urgency"`
	SuggestedAction  string   `json:"suggested_action,omitempty"`
	BlockedBy        []string `json:"blocked_by,omitempty"`
	Risk             string   `json:"risk,omitempty"`
}

func EvaluateAdvice(s Snapshot) Advice {
	if isFogHarbor(s.ScenarioID) {
		return evaluateFogHarborAdvice(s)
	}
	return evaluateObjectiveAdvice(s)
}

func RankActions(advice Advice, actions []orchestrator.SuggestedAction) []orchestrator.SuggestedAction {
	if advice.PrimaryObjective == "" || len(actions) < 2 {
		return actions
	}
	ranked := append([]orchestrator.SuggestedAction(nil), actions...)
	sort.SliceStable(ranked, func(i, j int) bool {
		return actionScore(advice, ranked[i]) > actionScore(advice, ranked[j])
	})
	return ranked
}

func evaluateObjectiveAdvice(s Snapshot) Advice {
	if s.Objective.Title == "" {
		return Advice{}
	}
	advice := Advice{
		PrimaryObjective: s.Objective.Title,
		Reason:           "当前阶段目标来自剧本 objective；优先选择能推进该目标的调查、对话或物品行动。",
		Urgency:          urgencyFromThreats(s.Threats),
		SuggestedAction:  firstNonEmpty(firstString(s.Objective.Steps), "选择案件板里与当前目标最接近的行动。"),
		BlockedBy:        objectiveBlocks(s.Objective),
	}
	if risk := highestRiskLabel(s.Threats); risk != "" {
		advice.Risk = risk
	}
	return advice
}

func actionScore(advice Advice, action orchestrator.SuggestedAction) int {
	text := strings.ToLower(action.Label + " " + action.Action.Text + " " + action.DisplayInput())
	score := 0
	keywords := adviceKeywords(advice)
	for _, token := range keywords {
		if strings.Contains(text, strings.ToLower(token)) {
			score += 10
		}
	}
	if special := fogHarborActionKindScore(advice, action); special != 0 || len(fogHarborAdviceKeywords(advice)) > 0 {
		return score + special
	}
	switch action.Action.Kind {
	case orchestrator.IntentInvestigate:
		if containsAny(text, "调查", "查看", "搜索", "线索", "证据") {
			score += 2
		}
	case orchestrator.IntentTalk:
		if containsAny(text, "询问", "追问", "交谈", "信任", "证言") {
			score += 2
		}
	case orchestrator.IntentUseItem:
		if len(advice.BlockedBy) > 0 {
			score += 1
		}
	}
	return score
}

func adviceKeywords(advice Advice) []string {
	if special := fogHarborAdviceKeywords(advice); len(special) > 0 {
		return special
	}
	var out []string
	out = append(out, strings.Fields(advice.PrimaryObjective)...)
	out = append(out, strings.Fields(advice.SuggestedAction)...)
	out = append(out, advice.BlockedBy...)
	return out
}

func urgencyFromThreats(threats []scenario.ThreatStatus) Urgency {
	maxSeverity := 0
	for _, threat := range threats {
		if threat.Severity > maxSeverity {
			maxSeverity = threat.Severity
		}
	}
	switch {
	case maxSeverity >= 3:
		return UrgencyCritical
	case maxSeverity >= 1:
		return UrgencyWarning
	default:
		return UrgencyNormal
	}
}

func highestRiskLabel(threats []scenario.ThreatStatus) string {
	var best scenario.ThreatStatus
	for _, threat := range threats {
		if threat.Severity > best.Severity {
			best = threat
		}
	}
	if best.ID == "" {
		return ""
	}
	return strings.TrimSpace(best.Name + "：" + best.StateLabel)
}

func objectiveBlocks(objective scenario.Objective) []string {
	var out []string
	for _, step := range objective.Steps {
		step = strings.TrimSpace(step)
		if step != "" {
			out = append(out, step)
		}
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func firstString(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func containsAny(text string, tokens ...string) bool {
	for _, token := range tokens {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
