package orchestrator

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

const encodedActionPrefix = "[action]"

type PlayerAction struct {
	Kind    TurnIntent     `json:"kind"`
	Raw     string         `json:"raw"`
	Text    string         `json:"text"`
	Source  ActionSource   `json:"source,omitempty"`
	Targets []ActionTarget `json:"targets,omitempty"`
}

type ActionSource struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type ActionTarget struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

func parsePlayerAction(ctx context.Context, repo *store.Repository, saveID string, scn *scenario.Scenario, input string) PlayerAction {
	raw := strings.TrimSpace(input)
	if action, ok := DecodePlayerAction(raw); ok {
		if action.Raw == "" {
			action.Raw = raw
		}
		if action.Text == "" {
			action.Text = normalizeActionText(action.Raw)
		}
		return action
	}
	action := PlayerAction{
		Kind: IntentUnknown,
		Raw:  raw,
		Text: normalizeActionText(raw),
	}
	if action.Text == "" {
		return action
	}
	if strings.HasPrefix(action.Text, "[hint]") {
		action.Kind = IntentInvestigate
		return action
	}
	if strings.HasPrefix(action.Text, "[all]") {
		action.Kind = IntentTalk
		action.Text = strings.TrimSpace(strings.TrimPrefix(action.Text, "[all]"))
		return action
	}
	if target, ok := bracketTarget(action.Text, "talk"); ok {
		action.Kind = IntentTalk
		action.Text = strings.TrimSpace(strings.TrimPrefix(action.Text, "[talk:"+target+"]"))
		action.Targets = append(action.Targets, ActionTarget{Kind: "npc", ID: target})
		return hydrateActionTargets(ctx, repo, action)
	}

	sv, err := repo.GetSave(ctx, saveID)
	if err != nil {
		action.Kind = inferIntent(action.Text, nil)
		return action
	}
	action.Targets = detectActionTargets(ctx, repo, saveID, scn, sv.CurrentLocationID, action.Text)
	action.Kind = inferActionKind(action.Text, action.Targets)
	return action
}

func EncodePlayerAction(action PlayerAction) string {
	b, err := json.Marshal(action)
	if err != nil {
		return strings.TrimSpace(action.Raw)
	}
	return encodedActionPrefix + string(b)
}

func DecodePlayerAction(input string) (PlayerAction, bool) {
	if !strings.HasPrefix(input, encodedActionPrefix) {
		return PlayerAction{}, false
	}
	var action PlayerAction
	if err := json.Unmarshal([]byte(strings.TrimPrefix(input, encodedActionPrefix)), &action); err != nil {
		return PlayerAction{}, false
	}
	action.Raw = strings.TrimSpace(action.Raw)
	action.Text = normalizeActionText(action.Text)
	if action.Text == "" {
		action.Text = normalizeActionText(action.Raw)
	}
	return action, true
}

func normalizeActionText(input string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(input)), " ")
}

func bracketTarget(text, name string) (string, bool) {
	prefix := "[" + name + ":"
	if !strings.HasPrefix(text, prefix) {
		return "", false
	}
	end := strings.Index(text, "]")
	if end <= len(prefix) {
		return "", false
	}
	return strings.TrimSpace(text[len(prefix):end]), true
}

func detectActionTargets(ctx context.Context, repo *store.Repository, saveID string, scn *scenario.Scenario, currentLocationID, text string) []ActionTarget {
	targets := []ActionTarget{}
	if scn != nil {
		for _, loc := range scn.Locations {
			if entityMentioned(text, loc.ID, loc.Name) {
				targets = append(targets, ActionTarget{Kind: "location", ID: loc.ID, Name: loc.Name})
			}
		}
		for _, npc := range scn.NPCs {
			if entityMentioned(text, npc.ID, npc.Name) {
				targets = append(targets, ActionTarget{Kind: "npc", ID: npc.ID, Name: npc.Name})
			}
		}
		for _, item := range scn.Items {
			if entityMentioned(text, item.ID, item.Name) {
				targets = append(targets, ActionTarget{Kind: "item", ID: item.ID, Name: item.Name})
			}
		}
		for _, clue := range scn.Clues {
			if entityMentioned(text, clue.ID, clue.Description) {
				targets = append(targets, ActionTarget{Kind: "clue", ID: clue.ID})
			}
		}
	}
	if len(targets) > 0 {
		return dedupeTargets(targets)
	}
	if currentLocationID != "" {
		if npcs, err := repo.ListNPCsAtLocation(ctx, saveID, currentLocationID); err == nil {
			for _, npc := range npcs {
				if entityMentioned(text, npc.ID, npc.Name) {
					targets = append(targets, ActionTarget{Kind: "npc", ID: npc.ID, Name: npc.Name})
				}
			}
		}
	}
	if items, err := repo.ListItems(ctx, saveID); err == nil {
		for _, item := range items {
			if item.Destroyed {
				continue
			}
			if entityMentioned(text, item.ID, item.Name) {
				targets = append(targets, ActionTarget{Kind: "item", ID: item.ID, Name: item.Name})
			}
		}
	}
	return dedupeTargets(targets)
}

func hydrateActionTargets(ctx context.Context, repo *store.Repository, action PlayerAction) PlayerAction {
	for i, target := range action.Targets {
		if target.Kind != "npc" || target.ID == "" || target.Name != "" {
			continue
		}
		if npc, err := repo.GetNPC(ctx, target.ID); err == nil {
			action.Targets[i].Name = npc.Name
		}
	}
	return action
}

func inferActionKind(text string, targets []ActionTarget) TurnIntent {
	if containsAny(text, "结案", "报告", "真凶", "指认") {
		return IntentReport
	}
	if containsAny(text, "去", "前往", "进入", "离开", "返回", "到") && hasTargetKind(targets, "location") {
		return IntentMove
	}
	if containsAny(text, "问", "说", "交谈", "追问", "询问", "告诉") || hasTargetKind(targets, "npc") {
		return IntentTalk
	}
	if containsAny(text, "使用", "点亮", "拿", "交给", "给", "打开") || hasTargetKind(targets, "item") {
		return IntentUseItem
	}
	if containsAny(text, "对峙", "攻击", "阻止", "制服", "威胁") {
		return IntentConfront
	}
	if containsAny(text, "查", "搜", "观察", "阅读", "查看", "调查", "寻找", "听", "闻") {
		return IntentInvestigate
	}
	if hasTargetKind(targets, "location") {
		return IntentMove
	}
	return IntentUnknown
}

func entityMentioned(text, id, name string) bool {
	text = strings.ToLower(text)
	for _, value := range []string{id, name} {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		if strings.Contains(text, value) {
			return true
		}
		if hasSignificantNameFragment(text, value) {
			return true
		}
	}
	return false
}

func hasSignificantNameFragment(text, name string) bool {
	if !containsNonASCII(name) {
		return false
	}
	runes := []rune(name)
	if len(runes) < 3 {
		return false
	}
	for size := min(4, len(runes)); size >= 2; size-- {
		for i := 0; i+size <= len(runes); i++ {
			part := string(runes[i : i+size])
			if isWeakEntityFragment(part) {
				continue
			}
			if strings.Contains(text, part) {
				return true
			}
		}
	}
	return false
}

func containsNonASCII(value string) bool {
	for _, r := range value {
		if r > 127 {
			return true
		}
	}
	return false
}

func isWeakEntityFragment(fragment string) bool {
	switch fragment {
	case "雾港", "清晨", "当前", "调查", "异常", "线索", "失踪", "记录":
		return true
	default:
		return false
	}
}

func containsAny(text string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func hasTargetKind(targets []ActionTarget, kind string) bool {
	for _, target := range targets {
		if target.Kind == kind {
			return true
		}
	}
	return false
}

func dedupeTargets(in []ActionTarget) []ActionTarget {
	out := make([]ActionTarget, 0, len(in))
	seen := map[string]bool{}
	for _, target := range in {
		key := target.Kind + ":" + target.ID
		if target.ID == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, target)
	}
	return out
}
