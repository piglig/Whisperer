package fogharbor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
)

func TestEvaluateAdviceOpeningEvidence(t *testing.T) {
	advice := EvaluateAdvice(DirectorSnapshot{ScenarioID: "fog_harbor"})

	assert.Equal(t, "确认露西失踪的第一现场", advice.PrimaryObjective)
	assert.Equal(t, UrgencyNormal, advice.Urgency)
	assert.Contains(t, advice.SuggestedAction, "码头")
}

func TestEvaluateAdviceProtectAnna(t *testing.T) {
	advice := EvaluateAdvice(DirectorSnapshot{
		ScenarioID: "fog_harbor",
		Turn:       18,
		FoundClues: map[string]bool{
			"blood_letter": true,
			"tide_chart":   true,
			"anna_warning": true,
		},
		Threats: []scenario.ThreatStatus{{ID: "anna_danger", Name: "安娜危险", StateLabel: "高危", Severity: 3}},
	})

	assert.Equal(t, "保护安娜", advice.PrimaryObjective)
	assert.Equal(t, UrgencyCritical, advice.Urgency)
	assert.Contains(t, advice.Risk, "安娜危险")
}

func TestEvaluateAdviceReefEvidence(t *testing.T) {
	advice := EvaluateAdvice(DirectorSnapshot{
		ScenarioID: "fog_harbor",
		FoundClues: map[string]bool{
			"blood_letter":   true,
			"tide_chart":     true,
			"ledger":         true,
			"parish_record":  true,
			"anna_warning":   true,
			"reef_carvings":  false,
			"sacrifice_room": false,
		},
		FiredTriggers: map[string]bool{"culprit_confronted": true},
	})

	assert.Equal(t, "进入礁洞取得非人证据", advice.PrimaryObjective)
	assert.Contains(t, advice.BlockedBy, "reef_carvings")
	assert.Contains(t, advice.BlockedBy, "sacrifice_chamber")
}

func TestEvaluateAdviceConfrontCulprit(t *testing.T) {
	advice := EvaluateAdvice(DirectorSnapshot{
		ScenarioID: "fog_harbor",
		FoundClues: map[string]bool{
			"blood_letter":      true,
			"tide_chart":        true,
			"ledger":            true,
			"parish_record":     true,
			"reef_carvings":     true,
			"sacrifice_chamber": true,
		},
	})

	assert.Equal(t, "对峙当代执行者", advice.PrimaryObjective)
	assert.Equal(t, UrgencyCritical, advice.Urgency)
}

func TestRankActionsPromotesDirectorRelevantAction(t *testing.T) {
	advice := Advice{PrimaryObjective: "拿到账册和出诊链条"}
	actions := []orchestrator.SuggestedAction{
		{
			Label: "查看公告栏",
			Action: orchestrator.PlayerAction{
				Kind: orchestrator.IntentInvestigate,
				Text: "查看公告栏",
			},
		},
		{
			Label: "询问玛丽莎：试探柜台账册",
			Action: orchestrator.PlayerAction{
				Kind: orchestrator.IntentTalk,
				Text: "试探玛丽莎柜台后的账册。",
			},
		},
	}

	ranked := RankActions(advice, actions)

	assert.Equal(t, "询问玛丽莎：试探柜台账册", ranked[0].Label)
	assert.Equal(t, "查看公告栏", actions[0].Label, "RankActions must not mutate input order")
}

func TestRankActionsKeepsOrderWithoutAdvice(t *testing.T) {
	actions := []orchestrator.SuggestedAction{{Label: "A"}, {Label: "B"}}

	ranked := RankActions(Advice{}, actions)

	assert.Equal(t, actions, ranked)
}
