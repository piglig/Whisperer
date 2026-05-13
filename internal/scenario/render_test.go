package scenario

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
