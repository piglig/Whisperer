package rules

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRollSkill_BasicCoverage 用大量随机种子覆盖所有 degree 分支。
func TestRollSkill_BasicCoverage(t *testing.T) {
	seenDegrees := map[Degree]bool{}
	for seed := uint64(1); seed <= 5000; seed++ {
		r := RollSkill("Spot Hidden", 50, DifficultyRegular, seededRand(seed), 0, 0)
		seenDegrees[r.Degree] = true
		assert.GreaterOrEqual(t, r.Roll, 1)
		assert.LessOrEqual(t, r.Roll, 100)
	}
	for _, d := range []Degree{
		DegreeCriticalSuccess, DegreeExtremeSuccess, DegreeHardSuccess,
		DegreeRegularSuccess, DegreeFailure, DegreeFumble,
	} {
		assert.True(t, seenDegrees[d], "expected to see degree %s in 5000 seeds", d)
	}
}

// TestRollSkill_SuccessSemantics 验证 difficulty 与 success 的对应关系。
func TestRollSkill_SuccessSemantics(t *testing.T) {
	for seed := uint64(1); seed <= 1000; seed++ {
		r := RollSkill("X", 70, DifficultyRegular, seededRand(seed), 0, 0)
		switch r.Degree {
		case DegreeCriticalSuccess, DegreeExtremeSuccess, DegreeHardSuccess, DegreeRegularSuccess:
			assert.True(t, r.Success)
		case DegreeFailure, DegreeFumble:
			assert.False(t, r.Success)
		}

		hr := RollSkill("X", 70, DifficultyHard, seededRand(seed), 0, 0)
		switch hr.Degree {
		case DegreeCriticalSuccess, DegreeExtremeSuccess, DegreeHardSuccess:
			assert.True(t, hr.Success)
		default:
			assert.False(t, hr.Success)
		}

		er := RollSkill("X", 70, DifficultyExtreme, seededRand(seed), 0, 0)
		switch er.Degree {
		case DegreeCriticalSuccess, DegreeExtremeSuccess:
			assert.True(t, er.Success)
		default:
			assert.False(t, er.Success)
		}
	}
}

// TestRollSkill_FumbleBoundary 验证 ≥50 / <50 的 fumble 边界差异。
func TestRollSkill_FumbleBoundary(t *testing.T) {
	// roll == 100 总是 fumble；< 50 时 96-99 也 fumble。
	for _, sv := range []int{30, 49, 50, 80} {
		var fumbleAt95to99 bool
		// 用 100 作为强制 fumble 的等效结果不易直接构造（rng 状态不可控到 1d10 级别）
		// 这里用 classifyDegree 做单元粒度断言更精确。
		got := classifyDegree(100, sv)
		assert.Equal(t, DegreeFumble, got, "100 must fumble at %d", sv)
		got = classifyDegree(96, sv)
		if sv < 50 {
			assert.Equal(t, DegreeFumble, got)
			fumbleAt95to99 = true
		} else {
			assert.NotEqual(t, DegreeFumble, got)
		}
		_ = fumbleAt95to99
	}
}

func TestRollSkill_DegreeBoundaries(t *testing.T) {
	// value=80 → extreme=80/5=16, hard=80/2=40, regular=80
	require.Equal(t, DegreeCriticalSuccess, classifyDegree(1, 80))
	require.Equal(t, DegreeExtremeSuccess, classifyDegree(16, 80))
	require.Equal(t, DegreeHardSuccess, classifyDegree(17, 80))
	require.Equal(t, DegreeHardSuccess, classifyDegree(40, 80))
	require.Equal(t, DegreeRegularSuccess, classifyDegree(41, 80))
	require.Equal(t, DegreeRegularSuccess, classifyDegree(80, 80))
	require.Equal(t, DegreeFailure, classifyDegree(81, 80))
	require.Equal(t, DegreeFailure, classifyDegree(99, 80))
	require.Equal(t, DegreeFumble, classifyDegree(100, 80))

	// value=30（<50）→ fumble window 96-100
	require.Equal(t, DegreeFumble, classifyDegree(96, 30))
	require.Equal(t, DegreeFumble, classifyDegree(99, 30))
	require.Equal(t, DegreeFumble, classifyDegree(100, 30))
	require.Equal(t, DegreeFailure, classifyDegree(95, 30))
}

// TestRollSkill_BonusDicePicksLowest 大量样本下，bonus 骰得到的 roll 平均更小。
func TestRollSkill_BonusDicePicksLowest(t *testing.T) {
	const N = 2000
	sumPlain, sumBonus := 0, 0
	for seed := uint64(1); seed <= N; seed++ {
		sumPlain += RollSkill("X", 50, DifficultyRegular, seededRand(seed), 0, 0).Roll
		sumBonus += RollSkill("X", 50, DifficultyRegular, seededRand(seed), 1, 0).Roll
	}
	avgPlain := float64(sumPlain) / N
	avgBonus := float64(sumBonus) / N
	assert.Less(t, avgBonus, avgPlain, "bonus dice should pick lower tens digit on average; plain=%.2f bonus=%.2f", avgPlain, avgBonus)
}

func TestRollSkill_PenaltyDicePicksHighest(t *testing.T) {
	const N = 2000
	sumPlain, sumPenalty := 0, 0
	for seed := uint64(1); seed <= N; seed++ {
		sumPlain += RollSkill("X", 50, DifficultyRegular, seededRand(seed), 0, 0).Roll
		sumPenalty += RollSkill("X", 50, DifficultyRegular, seededRand(seed), 0, 1).Roll
	}
	assert.Greater(t, float64(sumPenalty)/N, float64(sumPlain)/N)
}

func TestRollSkill_BonusPenaltyCancel(t *testing.T) {
	// 同时给 bonus=1 和 penalty=1 → 抵消，等价于 plain。
	for seed := uint64(1); seed <= 500; seed++ {
		plain := RollSkill("X", 50, DifficultyRegular, seededRand(seed), 0, 0).Roll
		canceled := RollSkill("X", 50, DifficultyRegular, seededRand(seed), 1, 1).Roll
		assert.Equal(t, plain, canceled, "seed=%d", seed)
	}
}

func TestRollSkill_ValueClamping(t *testing.T) {
	r := RollSkill("X", -10, DifficultyRegular, seededRand(1), 0, 0)
	assert.Equal(t, 0, r.SkillValue)
	r = RollSkill("X", 200, DifficultyRegular, seededRand(1), 0, 0)
	assert.Equal(t, 99, r.SkillValue)
}

func TestRollSkill_DiceClamping(t *testing.T) {
	r := RollSkill("X", 50, DifficultyRegular, seededRand(1), -5, -5)
	assert.Equal(t, 0, r.BonusDice)
	assert.Equal(t, 0, r.PenaltyDice)

	r = RollSkill("X", 50, DifficultyRegular, seededRand(1), 99, 99)
	assert.Equal(t, MaxBonusPenaltyDice, r.BonusDice)
	assert.Equal(t, MaxBonusPenaltyDice, r.PenaltyDice)
}

func TestRollSkill_Threshold(t *testing.T) {
	r := RollSkill("X", 80, DifficultyRegular, seededRand(1), 0, 0)
	assert.Equal(t, 80, r.Threshold)
	r = RollSkill("X", 80, DifficultyHard, seededRand(1), 0, 0)
	assert.Equal(t, 40, r.Threshold)
	r = RollSkill("X", 80, DifficultyExtreme, seededRand(1), 0, 0)
	assert.Equal(t, 16, r.Threshold)
}
