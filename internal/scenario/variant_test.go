package scenario

import (
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectVariant_NoVariantsReturnsClone(t *testing.T) {
	base := &Scenario{
		ID:        "tiny",
		Title:     "T",
		Locations: []SLocation{{ID: "a", Name: "A", Description: "x"}},
		Start:     Start{Location: "a"},
		KeyClues:  []string{},
	}
	eff, vid, err := SelectVariant(base, nil)
	require.NoError(t, err)
	assert.Equal(t, "", vid)
	require.NotNil(t, eff)
	// 修改 eff 不应影响 base
	eff.Title = "mutated"
	assert.Equal(t, "T", base.Title)
}

func TestSelectVariant_ByID(t *testing.T) {
	base, err := LoadBundled("fog_harbor")
	require.NoError(t, err)

	for _, id := range []string{"vance_executes", "calvin_directs", "rourke_runs"} {
		eff, vid, err := SelectVariantByID(base, id)
		require.NoError(t, err, id)
		assert.Equal(t, id, vid)
		require.NotNil(t, eff)
		assert.NotEmpty(t, eff.Culprit)
		assert.Empty(t, eff.Variants, "effective scenario should have variants cleared")
		// effective scenario 必须仍能通过完整校验
		require.NoError(t, Validate(eff), "%s merged should pass Validate", id)
	}

	_, _, err = SelectVariantByID(base, "no_such_variant")
	assert.Error(t, err)
}

func TestSelectVariant_DeterministicWithSeed(t *testing.T) {
	base, err := LoadBundled("fog_harbor")
	require.NoError(t, err)

	pickWith := func(seed uint64) string {
		rng := rand.New(rand.NewPCG(seed, 1))
		_, vid, err := SelectVariant(base, rng)
		require.NoError(t, err)
		return vid
	}
	// 同 seed 必定同结果
	assert.Equal(t, pickWith(42), pickWith(42))
	// 多 seed 至少能选到 2 个不同 variant（验证权重不卡死）
	seen := map[string]bool{}
	for s := uint64(1); s < 50; s++ {
		seen[pickWith(s)] = true
	}
	assert.GreaterOrEqual(t, len(seen), 2, "different seeds should yield different variants over 50 trials")
}

func TestMergeVariant_PatchesNPCSecretsAndClues(t *testing.T) {
	base, err := LoadBundled("fog_harbor")
	require.NoError(t, err)
	var rourkeRuns *Variant
	for i := range base.Variants {
		if base.Variants[i].ID == "rourke_runs" {
			rourkeRuns = &base.Variants[i]
			break
		}
	}
	require.NotNil(t, rourkeRuns)

	eff := MergeVariant(base, rourkeRuns)
	require.NotNil(t, eff)

	// rourke_runs 重写 rourke 的 secret 为"当代主谋"
	for _, n := range eff.NPCs {
		if n.ID == "rourke" {
			assert.Contains(t, n.Secret, "主谋")
		}
		if n.ID == "vance" {
			assert.Contains(t, n.Secret, "威胁")
		}
	}
	// rourke_runs 改写 rourke_bribe 的描述
	for _, c := range eff.Clues {
		if c.ID == "rourke_bribe" {
			assert.Contains(t, c.Description, "杠杆")
		}
	}

	// base 不被修改
	for _, n := range base.NPCs {
		if n.ID == "rourke" {
			assert.NotContains(t, n.Secret, "当代主谋", "base.rourke.secret should remain default")
		}
	}
}

func TestMergeVariant_TriggerOverride(t *testing.T) {
	base, err := LoadBundled("fog_harbor")
	require.NoError(t, err)
	var rourkeRuns *Variant
	for i := range base.Variants {
		if base.Variants[i].ID == "rourke_runs" {
			rourkeRuns = &base.Variants[i]
			break
		}
	}
	require.NotNil(t, rourkeRuns)

	eff := MergeVariant(base, rourkeRuns)
	for _, t2 := range eff.Triggers {
		if t2.ID == "culprit_confronted" {
			// rourke_runs 把对峙触发器改为 rourke_bribe + station
			require.Len(t, t2.When.All, 2)
			gotClue := false
			gotLoc := false
			for _, sub := range t2.When.All {
				if sub.ClueFound == "rourke_bribe" {
					gotClue = true
				}
				if sub.CurrentLocation == "station" {
					gotLoc = true
				}
			}
			assert.True(t, gotClue, "culprit_confronted should require rourke_bribe under rourke_runs")
			assert.True(t, gotLoc, "culprit_confronted should require station under rourke_runs")
		}
	}
}
