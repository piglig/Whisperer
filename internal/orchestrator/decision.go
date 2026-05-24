package orchestrator

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/zhuzhenwu/whisperer/internal/agent"
	"github.com/zhuzhenwu/whisperer/internal/orchestrator/sla"
)

type TurnIntent string

const (
	IntentUnknown     TurnIntent = "unknown"
	IntentInvestigate TurnIntent = "investigate"
	IntentTalk        TurnIntent = "talk"
	IntentMove        TurnIntent = "move"
	IntentUseItem     TurnIntent = "use_item"
	IntentConfront    TurnIntent = "confront"
	IntentReport      TurnIntent = "report"
)

type TurnDecision struct {
	Intent         TurnIntent       `json:"intent"`
	PlayerInput    string           `json:"player_input"`
	Action         PlayerAction     `json:"action"`
	ActionDecision ActionDecision   `json:"action_decision"`
	StateChanges   []DecisionChange `json:"state_changes,omitempty"`
	Mechanics      []DecisionAction `json:"mechanics,omitempty"`
	Checks         []DecisionCheck  `json:"checks,omitempty"`
}

type ActionDecisionStatus string

const (
	ActionAllowed    ActionDecisionStatus = "allowed"
	ActionBlocked    ActionDecisionStatus = "blocked"
	ActionRedirected ActionDecisionStatus = "redirected"
)

type ActionDecision struct {
	Status             ActionDecisionStatus `json:"status"`
	ReasonCode         string               `json:"reason_code,omitempty"`
	PlayerFacingReason string               `json:"player_facing_reason,omitempty"`
	DebugReason        string               `json:"debug_reason,omitempty"`
	RequiredClues      []string             `json:"required_clues,omitempty"`
	SuggestedActions   []string             `json:"suggested_actions,omitempty"`
}

type DecisionChange struct {
	Kind   string `json:"kind"`
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type DecisionAction struct {
	Tool    string `json:"tool"`
	Success bool   `json:"success"`
	Target  string `json:"target,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

type DecisionCheck struct {
	Code    string `json:"code"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

func buildTurnDecision(action PlayerAction, trace agent.TurnTrace, summary TurnSummary, report sla.Report) TurnDecision {
	decision := TurnDecision{
		Intent:         action.Kind,
		PlayerInput:    strings.TrimSpace(action.Raw),
		Action:         action,
		ActionDecision: ActionDecision{Status: ActionAllowed},
	}
	if decision.Intent == IntentUnknown {
		decision.Intent = inferIntent(action.Text, trace.ToolCalls)
	}
	decision.StateChanges = append(decision.StateChanges, decisionChangesFromSummary(summary)...)
	decision.Mechanics = decisionActionsFromTrace(trace.ToolCalls)
	decision.Checks = append(decision.Checks, decisionChecksFromTools(trace.ToolCalls)...)
	decision.Checks = append(decision.Checks, decisionChecksFromSLA(report)...)
	if len(decision.Checks) == 0 {
		decision.Checks = append(decision.Checks, DecisionCheck{
			Code:   "rules_applied",
			Passed: true,
		})
	}
	return decision
}

func inferIntent(input string, calls []agent.ToolCall) TurnIntent {
	for _, call := range calls {
		if call.IsError {
			continue
		}
		switch call.Name {
		case "transition_location":
			return IntentMove
		case "mark_clue_found", "get_location", "list_found_clues":
			return IntentInvestigate
		case "npc_speak", "get_npc", "list_npcs_at_location", "update_npc_relation":
			return IntentTalk
		case "move_item", "destroy_item":
			return IntentUseItem
		case "opposed_roll", "roll_damage", "kill_npc":
			return IntentConfront
		}
	}
	normalized := strings.TrimSpace(input)
	switch {
	case strings.Contains(normalized, "结案") || strings.Contains(normalized, "报告") || strings.Contains(normalized, "真凶"):
		return IntentReport
	case strings.Contains(normalized, "去") || strings.Contains(normalized, "前往") || strings.Contains(normalized, "进入") || strings.Contains(normalized, "离开"):
		return IntentMove
	case strings.Contains(normalized, "问") || strings.Contains(normalized, "说") || strings.Contains(normalized, "交谈") || strings.Contains(normalized, "追问"):
		return IntentTalk
	case strings.Contains(normalized, "使用") || strings.Contains(normalized, "拿") || strings.Contains(normalized, "给"):
		return IntentUseItem
	case strings.Contains(normalized, "对峙") || strings.Contains(normalized, "攻击") || strings.Contains(normalized, "阻止"):
		return IntentConfront
	case strings.Contains(normalized, "查") || strings.Contains(normalized, "搜") || strings.Contains(normalized, "观察") || strings.Contains(normalized, "阅读"):
		return IntentInvestigate
	default:
		return IntentUnknown
	}
}

func decisionChangesFromSummary(summary TurnSummary) []DecisionChange {
	changes := []DecisionChange{}
	if summary.LocationChange != nil {
		changes = append(changes, DecisionChange{
			Kind: "location",
			From: summary.LocationChange.From,
			To:   summary.LocationChange.To,
		})
	}
	if summary.TimeChange != nil {
		changes = append(changes, DecisionChange{
			Kind: "time",
			From: summary.TimeChange.From,
			To:   summary.TimeChange.To,
		})
	}
	if summary.StageChange != nil {
		changes = append(changes, DecisionChange{
			Kind: "stage",
			From: summary.StageChange.From,
			To:   summary.StageChange.To,
		})
	}
	for _, clue := range summary.NewClues {
		changes = append(changes, DecisionChange{
			Kind:   "clue",
			ID:     clue.ID,
			Detail: clue.Description,
		})
	}
	for _, npc := range summary.NPCChanges {
		if npc.AliveChanged {
			changes = append(changes, DecisionChange{
				Kind: "npc_alive",
				ID:   npc.ID,
				Name: npc.Name,
				From: boolState(npc.AliveFrom),
				To:   boolState(npc.AliveTo),
			})
		}
		if npc.RelationFrom != npc.RelationTo {
			changes = append(changes, DecisionChange{
				Kind: "npc_relation",
				ID:   npc.ID,
				Name: npc.Name,
				From: signedInt(npc.RelationFrom),
				To:   signedInt(npc.RelationTo),
			})
		}
	}
	for _, threat := range summary.ThreatChanges {
		changes = append(changes, DecisionChange{
			Kind: "threat",
			ID:   threat.ID,
			Name: threat.Name,
			From: threat.From,
			To:   threat.To,
		})
	}
	for _, id := range summary.FiredTriggers {
		changes = append(changes, DecisionChange{
			Kind: "trigger",
			ID:   id,
		})
	}
	return changes
}

func decisionActionsFromTrace(calls []agent.ToolCall) []DecisionAction {
	actions := make([]DecisionAction, 0, len(calls))
	for _, call := range calls {
		action := DecisionAction{
			Tool:    call.Name,
			Success: !call.IsError,
			Target:  toolTarget(call),
			Detail:  toolDetail(call),
		}
		actions = append(actions, action)
	}
	return actions
}

func decisionChecksFromTools(calls []agent.ToolCall) []DecisionCheck {
	checks := []DecisionCheck{}
	for _, call := range calls {
		if !call.IsError {
			continue
		}
		checks = append(checks, DecisionCheck{
			Code:    "tool_rejected",
			Passed:  false,
			Message: call.Name + ": " + toolErrorMessage(call.Output),
		})
	}
	return checks
}

func decisionChecksFromSLA(report sla.Report) []DecisionCheck {
	checks := []DecisionCheck{}
	for _, violation := range report.Violations {
		checks = append(checks, DecisionCheck{
			Code:    string(violation.Code),
			Passed:  false,
			Message: violation.Message,
		})
	}
	if report.EndingForced {
		checks = append(checks, DecisionCheck{
			Code:    "ending_forced",
			Passed:  true,
			Message: "investigator state forces an ending",
		})
	}
	return checks
}

func toolTarget(call agent.ToolCall) string {
	var input map[string]any
	if err := json.Unmarshal(call.Input, &input); err != nil {
		return ""
	}
	for _, key := range []string{"location_id", "npc_id", "clue_id", "item_id", "investigator_id"} {
		if value, ok := input[key].(string); ok {
			return value
		}
	}
	return ""
}

func toolDetail(call agent.ToolCall) string {
	switch call.Name {
	case "roll_skill", "sanity_check", "opposed_roll", "roll_damage":
		return compactJSON(call.Output)
	case "mark_clue_found", "transition_location", "move_item", "destroy_item", "kill_npc", "update_npc_relation":
		if call.IsError {
			return toolErrorMessage(call.Output)
		}
		return "applied"
	default:
		if call.IsError {
			return toolErrorMessage(call.Output)
		}
		return ""
	}
}

func toolErrorMessage(raw json.RawMessage) string {
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &payload); err == nil && payload.Error != "" {
		return payload.Error
	}
	return compactJSON(raw)
}

func compactJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	return string(b)
}

func boolState(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func signedInt(value int) string {
	if value > 0 {
		return "+" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}
