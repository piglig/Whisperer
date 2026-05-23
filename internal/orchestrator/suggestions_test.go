package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

func TestSuggestActionsOrdersAndEncodesContextualActions(t *testing.T) {
	scn := &scenario.Scenario{
		Objectives: []scenario.Objective{{Stage: "opening", Title: "Start"}},
		Locations: []scenario.SLocation{{
			ID: "harbor", Name: "雾港码头",
			Leads: []string{"查看公告栏", "沿退潮线搜索", "询问码头工人", "第四条会被截断"},
		}},
		NPCs: []scenario.SNPC{{
			ID: "vance", Name: "范斯",
			DialogueOptions: []scenario.DialogueOption{{
				ID: "ask_lucy", Label: "问露西", Prompt: "询问范斯露西是否找过他。", Stages: []string{"opening"},
			}},
		}},
		Items: []scenario.SItem{{
			ID: "lantern", Name: "黄铜油灯", OwnerType: "location", OwnerID: "harbor",
			Actions: []scenario.ItemAction{{
				ID: "take", Label: "拿起", Prompt: "拿起黄铜油灯。", Stages: []string{"opening"}, Locations: []string{"harbor"}, OwnerTypes: []string{"location"},
			}},
		}},
	}

	actions := SuggestActions(SuggestionInput{
		Scenario: scn,
		Save:     store.Save{Stage: "opening"},
		Location: store.Location{ID: "harbor", Name: "雾港码头"},
		NPCs:     []store.NPC{{ID: "vance", Name: "范斯", LocationID: "harbor", Alive: true}},
		Items:    []store.Item{{ID: "lantern", Name: "黄铜油灯", OwnerType: store.OwnerLocation, OwnerID: "harbor"}},
		Limit:    5,
	})

	require.Len(t, actions, 5)
	assert.Equal(t, "查看公告栏", actions[0].Label)
	assert.Equal(t, "使用黄铜油灯：拿起", actions[3].Label)
	assert.Equal(t, "询问范斯：问露西", actions[4].Label)

	decoded, ok := DecodePlayerAction(actions[0].Input)
	require.True(t, ok)
	assert.Equal(t, IntentInvestigate, decoded.Kind)
	assert.Equal(t, "lead", decoded.Source.Kind)
	assert.Equal(t, "harbor:0", decoded.Source.ID)
	assert.Equal(t, "查看公告栏", actions[0].DisplayInput())
}

func TestSuggestActionsFallsBackToObserveLocation(t *testing.T) {
	actions := SuggestActions(SuggestionInput{
		Location: store.Location{ID: "room", Name: "空房间"},
	})

	require.Len(t, actions, 1)
	assert.Equal(t, "观察空房间，寻找异常痕迹或可调查的物件", actions[0].Label)
	assert.Equal(t, "fallback", actions[0].Action.Source.Kind)
}

func TestSuggestActionsFiltersByStageAndClues(t *testing.T) {
	scn := &scenario.Scenario{
		Objectives: []scenario.Objective{
			{Stage: "opening", Title: "Start"},
			{Stage: "confrontation", Title: "End"},
		},
		Locations: []scenario.SLocation{{ID: "room", Name: "Room"}},
		NPCs: []scenario.SNPC{{
			ID: "npc", Name: "NPC",
			DialogueOptions: []scenario.DialogueOption{
				{ID: "early", Label: "Early", Prompt: "early", Stages: []string{"opening"}},
				{ID: "late", Label: "Late", Prompt: "late", Stages: []string{"confrontation"}, RequiresClues: []string{"proof"}},
			},
		}},
	}

	actions := SuggestActions(SuggestionInput{
		Scenario: scn,
		Save:     store.Save{Stage: "confrontation"},
		Location: store.Location{ID: "room", Name: "Room"},
		NPCs:     []store.NPC{{ID: "npc", Name: "NPC"}},
		Clues:    []store.Clue{{ID: "proof", Found: true}},
	})

	require.Len(t, actions, 1)
	assert.Equal(t, "询问NPC：Late", actions[0].Label)
}
