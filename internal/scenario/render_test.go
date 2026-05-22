package scenario

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/store"
)

func TestRender_FogHarborProducesAllSections(t *testing.T) {
	base, err := LoadBundled("fog_harbor")
	require.NoError(t, err)
	eff, _, err := SelectVariantByID(base, "vance_executes")
	require.NoError(t, err)

	truth := RenderTruth(eff)
	assert.Contains(t, truth, "vance_executes")
	assert.Contains(t, truth, "深潜者")

	secrets := RenderNPCSecrets(eff)
	assert.Contains(t, secrets, "vance")
	assert.Contains(t, secrets, "father_calvin")

	knowledge := RenderNPCKnowledge(eff)
	assert.Contains(t, knowledge, "sacrifice_history")
	assert.Contains(t, knowledge, "关键词")

	atlas := RenderClueAtlas(eff)
	assert.Contains(t, atlas, "Tier 1")
	assert.Contains(t, atlas, "Tier 2")
	assert.Contains(t, atlas, "Tier 3")
	assert.Contains(t, atlas, "Red herring")
	assert.Contains(t, atlas, "marisa_exhusband")
}

func TestRenderOpeningBriefingIsPlayerFacing(t *testing.T) {
	base, err := LoadBundled("fog_harbor")
	require.NoError(t, err)
	eff, _, err := SelectVariantByID(base, "vance_executes")
	require.NoError(t, err)

	out := RenderOpeningBriefing(eff)

	assert.Contains(t, out, "你是从外地赶来的记者")
	assert.Contains(t, out, "清晨的渡船")
	assert.Contains(t, out, "当前目标")
	assert.Contains(t, out, "弄清露西失踪前最后去了哪里")
	assert.Contains(t, out, "可以从这些行动开始")
	assert.Contains(t, out, "查看公告栏")
	assert.Contains(t, out, "询问码头工人")
	assert.NotContains(t, out, "variant")
	assert.NotContains(t, out, "vance_executes")
	assert.NotContains(t, out, "剧本 id")
	assert.NotContains(t, out, "fog_harbor")
	assert.NotContains(t, out, "当前位置")
}

func TestRenderPlayerGuidance(t *testing.T) {
	base, err := LoadBundled("fog_harbor")
	require.NoError(t, err)

	out := RenderPlayerGuidance(base)

	assert.Contains(t, out, "## 目标")
	assert.Contains(t, out, "opening")
	assert.Contains(t, out, "## 地点行动提示")
	assert.Contains(t, out, "雾港码头")
	assert.Contains(t, out, "查看公告栏")
	assert.Contains(t, out, "## NPC 初见")
	assert.Contains(t, out, "海莲娜")
	assert.Contains(t, out, "请你听我说完")
	assert.Contains(t, out, "可问")
	assert.Contains(t, out, "失踪当晚")
	assert.Contains(t, out, "## 物品行动")
	assert.Contains(t, out, "黄铜油灯")
	assert.NotContains(t, out, "深潜者长老作为契约的非人方现身")
}

func TestDialogueOptionsForFiltersByStageAndClues(t *testing.T) {
	npc := SNPC{
		DialogueOptions: []DialogueOption{
			{ID: "open", Label: "开局", Prompt: "a", Stages: []string{"opening"}},
			{ID: "needs", Label: "证据", Prompt: "b", Stages: []string{"investigation"}, RequiresClues: []string{"ledger"}},
			{ID: "hidden", Label: "隐藏", Prompt: "c", SuppressIfClues: []string{"ledger"}},
		},
	}

	opening := DialogueOptionsFor(npc, "opening", nil)
	require.Len(t, opening, 2)
	assert.Equal(t, "open", opening[0].ID)
	assert.Equal(t, "hidden", opening[1].ID)

	investigation := DialogueOptionsFor(npc, "investigation", map[string]bool{"ledger": true})
	require.Len(t, investigation, 1)
	assert.Equal(t, "needs", investigation[0].ID)
}

func TestItemActionsForFiltersByState(t *testing.T) {
	item := SItem{
		Actions: []ItemAction{
			{ID: "take", Label: "拿起", Prompt: "a", Stages: []string{"opening"}, Locations: []string{"harbor"}, OwnerTypes: []string{"location"}},
			{ID: "use", Label: "使用", Prompt: "b", Stages: []string{"confrontation"}, OwnerTypes: []string{"investigator"}, RequiresClues: []string{"reef_carvings"}},
		},
	}

	opening := ItemActionsFor(item, store.Item{OwnerType: store.OwnerLocation}, "opening", "harbor", nil)
	require.Len(t, opening, 1)
	assert.Equal(t, "take", opening[0].ID)

	confront := ItemActionsFor(item, store.Item{OwnerType: store.OwnerInvestigator}, "confrontation", "reef_cave", map[string]bool{"reef_carvings": true})
	require.Len(t, confront, 1)
	assert.Equal(t, "use", confront[0].ID)

	destroyed := ItemActionsFor(item, store.Item{OwnerType: store.OwnerInvestigator, Destroyed: true}, "confrontation", "reef_cave", nil)
	assert.Empty(t, destroyed)
}

func TestBuildCaseReport(t *testing.T) {
	base, err := LoadBundled("fog_harbor")
	require.NoError(t, err)
	eff, variantID, err := SelectVariantByID(base, "vance_executes")
	require.NoError(t, err)
	ending := &Ending{ID: "solved", Kind: "success", Description: "结案"}

	report := BuildCaseReport(eff, CaseReportInput{
		Save:      store.Save{Stage: "confrontation"},
		Ending:    ending,
		VariantID: variantID,
		FoundClues: []store.Clue{
			{ID: "blood_letter", FoundAtTurn: 1},
			{ID: "ledger", FoundAtTurn: 4},
		},
		NPCs: []store.NPC{
			{ID: "vance", Name: "范斯医生", Alive: true, RelationToPlayer: -50},
			{ID: "anna", Name: "安娜", Alive: false, RelationToPlayer: 0},
		},
		TruthLimit: 80,
	})

	require.NotNil(t, report)
	assert.Equal(t, "vance_executes", report.VariantID)
	assert.Equal(t, "vance", report.CulpritID)
	assert.Equal(t, "范斯医生", report.CulpritName)
	assert.Len(t, report.FoundKeyClues, 2)
	assert.NotEmpty(t, report.MissingKeyClues)
	assert.Contains(t, report.EvidenceStatus, "缺口")
	assert.NotEmpty(t, report.TruthSummary)
}

func TestRender_NPCKnowledgeForSingleNPC(t *testing.T) {
	base, err := LoadBundled("fog_harbor")
	require.NoError(t, err)
	out := RenderNPCKnowledgeFor(base, "vance")
	assert.Contains(t, out, "sacrifice_history")
	assert.Contains(t, out, "[献祭")
	assert.Contains(t, out, "SAN 损失 0/1d3")

	// 不存在的 npc 返回空串
	assert.Equal(t, "", RenderNPCKnowledgeFor(base, "nobody"))
}

func TestRender_EmptyOnNil(t *testing.T) {
	assert.Equal(t, "", RenderTruth(nil))
	assert.Equal(t, "", RenderOpeningBriefing(nil))
	assert.Equal(t, "", RenderPlayerGuidance(nil))
	assert.Equal(t, "", RenderNPCSecrets(nil))
	assert.Equal(t, "", RenderNPCKnowledge(nil))
	assert.Equal(t, "", RenderClueAtlas(nil))
	assert.Equal(t, "", RenderNPCKnowledgeFor(nil, "x"))
}

func TestRender_TierZeroIsRedHerring(t *testing.T) {
	s := &Scenario{
		Clues: []SClue{
			{ID: "tier1", Tier: 1, Description: "main"},
			{ID: "rh", Tier: 0, Description: "fake"},
		},
	}
	atlas := RenderClueAtlas(s)
	assert.Contains(t, atlas, "Tier 1")
	assert.Contains(t, atlas, "Red herring")
	// red herring section comes after tier section
	rhIdx := strings.Index(atlas, "Red herring")
	t1Idx := strings.Index(atlas, "Tier 1")
	assert.Greater(t, rhIdx, t1Idx)
}
