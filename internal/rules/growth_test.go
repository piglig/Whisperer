package rules

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGrowthCheck_NoGrowAtCap(t *testing.T) {
	for seed := uint64(1); seed <= 50; seed++ {
		r := GrowthCheck("X", 99, seededRand(seed))
		assert.False(t, r.Grew)
		assert.Equal(t, 99, r.NewValue)
	}
}

func TestGrowthCheck_GrowthBounded(t *testing.T) {
	for seed := uint64(1); seed <= 1000; seed++ {
		r := GrowthCheck("X", 50, seededRand(seed))
		if r.Grew {
			assert.GreaterOrEqual(t, r.Increment, 1)
			assert.LessOrEqual(t, r.Increment, 10)
			assert.LessOrEqual(t, r.NewValue, MaxSkillValue)
			assert.Equal(t, r.OldValue+r.Increment, r.NewValue)
		} else {
			assert.Equal(t, r.OldValue, r.NewValue)
		}
	}
}

func TestGrowthCheck_LowSkillUsuallyGrows(t *testing.T) {
	grew := 0
	for seed := uint64(1); seed <= 1000; seed++ {
		r := GrowthCheck("X", 10, seededRand(seed))
		if r.Grew {
			grew++
		}
	}
	// oldValue=10 → 90% 期望成长率
	assert.Greater(t, grew, 800)
}

func TestSettleGrowth_DeterministicOrder(t *testing.T) {
	used := map[string]int{"Spot Hidden": 50, "Library Use": 70, "Dodge": 30}
	out1 := SettleGrowth(used, seededRand(42))
	out2 := SettleGrowth(used, seededRand(42))
	assert.Equal(t, out1, out2)

	names := make([]string, len(out1))
	for i, r := range out1 {
		names[i] = r.SkillName
	}
	// 字典序：Dodge < Library Use < Spot Hidden
	assert.Equal(t, []string{"Dodge", "Library Use", "Spot Hidden"}, names)
}

func TestSettleGrowth_Empty(t *testing.T) {
	out := SettleGrowth(nil, seededRand(1))
	assert.Empty(t, out)
}

func TestGrowthCheck_NearCap(t *testing.T) {
	// oldValue=95，确保增长被截到 99
	for seed := uint64(1); seed <= 500; seed++ {
		r := GrowthCheck("X", 95, seededRand(seed))
		if r.Grew {
			assert.LessOrEqual(t, r.NewValue, MaxSkillValue)
		}
	}
}
