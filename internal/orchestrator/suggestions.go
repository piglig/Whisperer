package orchestrator

import (
	"fmt"
	"strings"

	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

type SuggestedAction struct {
	Label  string       `json:"label"`
	Input  string       `json:"input"`
	Action PlayerAction `json:"action"`
}

type SuggestionInput struct {
	Scenario *scenario.Scenario
	Save     store.Save
	Stage    string
	Location store.Location
	NPCs     []store.NPC
	Items    []store.Item
	Clues    []store.Clue
	Limit    int
}

func SuggestActions(input SuggestionInput) []SuggestedAction {
	actions := []SuggestedAction{}
	if loc, ok := suggestionScenarioLocation(input.Scenario, input.Location.ID); ok {
		for i, lead := range loc.Leads {
			lead = strings.TrimSpace(lead)
			if lead == "" {
				continue
			}
			actions = append(actions, newSuggestedAction(IntentInvestigate, lead, lead,
				ActionSource{Kind: "lead", ID: fmt.Sprintf("%s:%d", loc.ID, i)},
				ActionTarget{Kind: "location", ID: loc.ID, Name: loc.Name},
			))
		}
	}
	if len(actions) > 3 {
		actions = actions[:3]
	}
	actions = append(actions, suggestItemActions(input)...)
	actions = append(actions, suggestDialogueActions(input)...)
	if len(actions) == 0 && input.Location.Name != "" {
		text := "观察" + input.Location.Name + "，寻找异常痕迹或可调查的物件"
		actions = append(actions, newSuggestedAction(IntentInvestigate, text, text,
			ActionSource{Kind: "fallback", ID: input.Location.ID},
			ActionTarget{Kind: "location", ID: input.Location.ID, Name: input.Location.Name},
		))
	}
	if len(actions) == 0 {
		return nil
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 5
	}
	if len(actions) > limit {
		actions = actions[:limit]
	}
	return actions
}

func (a SuggestedAction) DisplayInput() string {
	if strings.TrimSpace(a.Action.Text) != "" {
		return a.Action.Text
	}
	if strings.TrimSpace(a.Input) != "" {
		if action, ok := DecodePlayerAction(a.Input); ok {
			return nonEmpty(action.Text, action.Raw)
		}
		return a.Input
	}
	return a.Label
}

func suggestDialogueActions(input SuggestionInput) []SuggestedAction {
	if input.Scenario == nil || len(input.NPCs) == 0 {
		return nil
	}
	found := suggestionFoundClueSet(input.Clues)
	stage := suggestionStage(input)
	out := []SuggestedAction{}
	for _, npc := range input.NPCs {
		snpc, ok := suggestionScenarioNPC(input.Scenario, npc.ID)
		if !ok {
			continue
		}
		for _, opt := range scenario.DialogueOptionsFor(snpc, stage, found) {
			prompt := strings.TrimSpace(opt.Prompt)
			if prompt == "" {
				continue
			}
			name := nonEmpty(snpc.Name, npc.Name)
			out = append(out, newSuggestedAction(IntentTalk, fmt.Sprintf("询问%s：%s", name, opt.Label), prompt,
				ActionSource{Kind: "dialogue_option", ID: opt.ID},
				ActionTarget{Kind: "npc", ID: npc.ID, Name: name},
			))
		}
	}
	return out
}

func suggestItemActions(input SuggestionInput) []SuggestedAction {
	if input.Scenario == nil || len(input.Items) == 0 {
		return nil
	}
	found := suggestionFoundClueSet(input.Clues)
	stage := suggestionStage(input)
	out := []SuggestedAction{}
	for _, state := range input.Items {
		sitem, ok := suggestionScenarioItem(input.Scenario, state.ID)
		if !ok {
			continue
		}
		for _, action := range scenario.ItemActionsFor(sitem, state, stage, input.Location.ID, found) {
			name := nonEmpty(sitem.Name, state.Name)
			out = append(out, newSuggestedAction(IntentUseItem, fmt.Sprintf("使用%s：%s", name, action.Label), action.Prompt,
				ActionSource{Kind: "item_action", ID: action.ID},
				ActionTarget{Kind: "item", ID: state.ID, Name: name},
			))
		}
	}
	return out
}

func newSuggestedAction(kind TurnIntent, label, text string, source ActionSource, targets ...ActionTarget) SuggestedAction {
	text = strings.TrimSpace(text)
	if text == "" {
		text = strings.TrimSpace(label)
	}
	action := PlayerAction{
		Kind:    kind,
		Raw:     text,
		Text:    text,
		Source:  source,
		Targets: targets,
	}
	return SuggestedAction{
		Label:  label,
		Input:  EncodePlayerAction(action),
		Action: action,
	}
}

func suggestionStage(input SuggestionInput) string {
	if input.Stage != "" {
		if stage := scenario.NormalizeStage(input.Scenario, input.Stage); stage != "" {
			return stage
		}
	}
	if input.Save.Stage != "" {
		if stage := scenario.NormalizeStage(input.Scenario, input.Save.Stage); stage != "" {
			return stage
		}
	}
	return scenario.DefaultStage
}

func suggestionFoundClueSet(clues []store.Clue) map[string]bool {
	found := make(map[string]bool, len(clues))
	for _, clue := range clues {
		found[clue.ID] = true
	}
	return found
}

func suggestionScenarioNPC(scn *scenario.Scenario, id string) (scenario.SNPC, bool) {
	if scn == nil {
		return scenario.SNPC{}, false
	}
	for _, npc := range scn.NPCs {
		if npc.ID == id {
			return npc, true
		}
	}
	return scenario.SNPC{}, false
}

func suggestionScenarioItem(scn *scenario.Scenario, id string) (scenario.SItem, bool) {
	if scn == nil {
		return scenario.SItem{}, false
	}
	for _, item := range scn.Items {
		if item.ID == id {
			return item, true
		}
	}
	return scenario.SItem{}, false
}

func suggestionScenarioLocation(scn *scenario.Scenario, id string) (scenario.SLocation, bool) {
	if scn == nil {
		return scenario.SLocation{}, false
	}
	for _, loc := range scn.Locations {
		if loc.ID == id {
			return loc, true
		}
	}
	return scenario.SLocation{}, false
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
