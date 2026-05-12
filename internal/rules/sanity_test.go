package rules

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanityCheck_ZeroLossOnSuccess(t *testing.T) {
	for seed := uint64(1); seed <= 100; seed++ {
		r, err := SanityCheck(60, "0", "1d10", seededRand(seed))
		require.NoError(t, err)
		if r.Success {
			assert.Equal(t, 0, r.Loss)
			assert.Equal(t, 60, r.NewSAN)
		} else {
			assert.Greater(t, r.Loss, 0)
			assert.Equal(t, 60-r.Loss, r.NewSAN)
		}
	}
}

func TestSanityCheck_FailLoss(t *testing.T) {
	// SAN=0 时所有 d100 (1..100) 都失败，必扣 lossFail
	for seed := uint64(1); seed <= 50; seed++ {
		r, err := SanityCheck(0, "0", "1d4", seededRand(seed))
		require.NoError(t, err)
		assert.False(t, r.Success)
		assert.GreaterOrEqual(t, r.Loss, 1)
		assert.LessOrEqual(t, r.Loss, 4)
		assert.Equal(t, 0, r.NewSAN)
	}
}

func TestSanityCheck_IndefiniteInsanity(t *testing.T) {
	// 失败 + 至少损失 5 → 不定性疯狂
	seenIndef := false
	for seed := uint64(1); seed <= 500; seed++ {
		r, err := SanityCheck(20, "0", "1d10", seededRand(seed))
		require.NoError(t, err)
		if r.Loss >= IndefiniteInsanityThreshold {
			assert.True(t, r.TriggeredIndefiniteInsanity)
			seenIndef = true
		} else {
			assert.False(t, r.TriggeredIndefiniteInsanity)
		}
	}
	assert.True(t, seenIndef, "should observe at least one indefinite insanity in 500 seeds")
}

func TestSanityCheck_TemporaryNotSetByRules(t *testing.T) {
	// rules 层不判定临时疯狂；保持 false。
	for seed := uint64(1); seed <= 100; seed++ {
		r, err := SanityCheck(50, "1", "1d4", seededRand(seed))
		require.NoError(t, err)
		assert.False(t, r.TriggeredTemporaryInsanity)
	}
}

func TestSanityCheck_NoNegativeSAN(t *testing.T) {
	r, err := SanityCheck(1, "0", "1d100", seededRand(7))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, r.NewSAN, 0)
}

func TestSanityCheck_NegativeCurrentSANClamped(t *testing.T) {
	r, err := SanityCheck(-10, "0", "1d4", seededRand(1))
	require.NoError(t, err)
	assert.Equal(t, 0, r.Threshold)
	assert.False(t, r.Success)
}

func TestSanityCheck_BadExpression(t *testing.T) {
	// SAN=99 → 几乎必然成功 → 使用 lossPass，故 lossPass 错误必然冒泡
	_, err := SanityCheck(99, "abc", "1d4", seededRand(1))
	require.Error(t, err)
	// SAN=0 → 必然失败 → 使用 lossFail
	_, err = SanityCheck(0, "0", "abc", seededRand(1))
	require.Error(t, err)
}
