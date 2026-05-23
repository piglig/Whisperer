package scenario

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func leadsTestScenario() *Scenario {
	return &Scenario{
		ID: "t",
		Locations: []SLocation{
			{ID: "church", Name: "圣安德鲁堂"},
			{ID: "pub", Name: "钨灯酒馆"},
		},
		NPCs: []SNPC{
			{ID: "calvin", Name: "卡尔文神父"},
			{ID: "marisa", Name: "玛丽莎"},
		},
		Clues: []SClue{
			{ID: "ledger", Description: "镇议会账册"},
			{ID: "parish_record", Description: "教区登记簿"},
			{ID: "tide_chart", Description: "潮汐表", Location: "lighthouse", Source: "orin"},
			{ID: "orphan", Description: "无路径线索"},
		},
		Triggers: []Trigger{
			{
				ID: "church_records_unlocked",
				When: Condition{All: []Condition{
					{CurrentLocation: "church"},
					{NPCRelationGT: &RelChk{NPC: "calvin", Value: 9}},
				}},
				Then: []Action{{MarkClueFound: &ActionMarkClueFound{ClueID: "parish_record"}}},
			},
			{
				ID: "pub_after_dark",
				When: Condition{All: []Condition{
					{CurrentLocation: "pub"},
					{TimeOfDay: "night"},
				}},
				Then: []Action{
					{MarkClueFound: &ActionMarkClueFound{ClueID: "ledger"}},
				},
			},
		},
	}
}

func TestLeadsForClue_TriggerWithHumanizedCondition(t *testing.T) {
	s := leadsTestScenario()
	leads := LeadsForClue(s, "parish_record")

	require.Len(t, leads, 1)
	assert.Equal(t, "trigger", leads[0].Kind)
	assert.Equal(t, "church_records_unlocked", leads[0].TriggerID)
	assert.Contains(t, leads[0].Hint, "圣安德鲁堂")
	assert.Contains(t, leads[0].Hint, "卡尔文神父")
	assert.Contains(t, leads[0].Hint, "好感 > 9")
}

func TestLeadsForClue_MergesLocationSourceAndTrigger(t *testing.T) {
	s := leadsTestScenario()
	leads := LeadsForClue(s, "tide_chart")
	require.Len(t, leads, 2)
	// trigger 没有，location 和 npc 都有 —— 应按顺序：location 在 npc 前
	assert.Equal(t, "location", leads[0].Kind)
	assert.Equal(t, "lighthouse", leads[0].LocationID)
	assert.Equal(t, "npc", leads[1].Kind)
	assert.Equal(t, "orin", leads[1].NPCID)
}

func TestLeadsForClue_PubLedger(t *testing.T) {
	s := leadsTestScenario()
	leads := LeadsForClue(s, "ledger")
	require.Len(t, leads, 1)
	assert.Equal(t, "trigger", leads[0].Kind)
	assert.Contains(t, leads[0].Hint, "钨灯酒馆")
	assert.Contains(t, leads[0].Hint, "夜晚")
}

func TestLeadsForClue_OrphanClueHasNoLeads(t *testing.T) {
	s := leadsTestScenario()
	assert.Empty(t, LeadsForClue(s, "orphan"))
}

func TestLeadsForClue_NilSafe(t *testing.T) {
	assert.Empty(t, LeadsForClue(nil, "x"))
	assert.Empty(t, LeadsForClue(leadsTestScenario(), ""))
}

func TestHumanizeCondition_AllConditionForms(t *testing.T) {
	s := leadsTestScenario()
	cases := []struct {
		name string
		cond Condition
		want []string // substrings required to appear
	}{
		{"current_location", Condition{CurrentLocation: "church"}, []string{"在", "圣安德鲁堂"}},
		{"location_in", Condition{LocationIn: []string{"church", "pub"}}, []string{"圣安德鲁堂", "钨灯酒馆", "或"}},
		{"location_visited", Condition{LocationVisited: "pub"}, []string{"曾去过", "钨灯酒馆"}},
		{"clue_found", Condition{ClueFound: "ledger"}, []string{"已发现", "镇议会账册"}},
		{"npc_dead", Condition{NPCDead: "marisa"}, []string{"玛丽莎", "死亡"}},
		{"npc_relation_gt", Condition{NPCRelationGT: &RelChk{NPC: "calvin", Value: 5}}, []string{"卡尔文神父", "好感 > 5"}},
		{"npc_relation_lt", Condition{NPCRelationLT: &RelChk{NPC: "calvin", Value: -3}}, []string{"卡尔文神父", "好感 < -3"}},
		{"time_of_day", Condition{TimeOfDay: "night"}, []string{"夜晚"}},
		{"turn_ge", Condition{TurnGE: 12}, []string{"第 12 回合"}},
		{"trigger_fired", Condition{TriggerFired: "lighthouse_storm"}, []string{"已触发", "lighthouse_storm"}},
		{"not", Condition{Not: &Condition{NPCDead: "marisa"}}, []string{"非"}},
		{"any", Condition{Any: []Condition{{CurrentLocation: "pub"}, {TimeOfDay: "night"}}}, []string{"或"}},
		{"empty", Condition{}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := humanizeCondition(s, tc.cond)
			if tc.want == nil {
				assert.Empty(t, got)
				return
			}
			for _, sub := range tc.want {
				assert.Contains(t, got, sub)
			}
		})
	}
}

func TestBuildCaseReport_MissingKeyCluesIncludeLeads(t *testing.T) {
	s := leadsTestScenario()
	s.KeyClues = []string{"ledger", "parish_record"}
	ending := &Ending{ID: "solved", Kind: "success"}

	report := BuildCaseReport(s, CaseReportInput{
		Ending: ending,
	})
	require.NotNil(t, report)
	require.Len(t, report.MissingKeyClues, 2)

	byID := map[string]ReportClue{}
	for _, c := range report.MissingKeyClues {
		byID[c.ID] = c
	}
	require.NotEmpty(t, byID["parish_record"].Leads)
	assert.Equal(t, "trigger", byID["parish_record"].Leads[0].Kind)
	assert.Contains(t, byID["parish_record"].Leads[0].Hint, "卡尔文神父")
}
