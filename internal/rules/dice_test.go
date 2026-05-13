package rules

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoll_ValidExpressions(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		seed     uint64
		wantTot  int   // 期望 Total（用 seededRand 锁定输出）
		wantRoll []int // 期望 Rolls
		wantMod  int   // 期望 Modifier
	}{
		{name: "constant", expr: "5", wantTot: 5, wantMod: 5},
		{name: "1d6 seed=1", expr: "1d6", seed: 1},
		{name: "2d6+3 seed=2", expr: "2d6+3", seed: 2},
		{name: "1d10-1 seed=3", expr: "1d10-1", seed: 3},
		{name: "spaces", expr: " 1d4 + 2 ", seed: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rng := seededRand(tt.seed)
			r, err := Roll(tt.expr, rng)
			require.NoError(t, err)
			// 总和一致性：Sum(Rolls) + Modifier == Total
			sum := 0
			for _, x := range r.Rolls {
				sum += x
			}
			assert.Equal(t, r.Total, sum+r.Modifier, "Total = Sum(Rolls) + Modifier")
		})
	}
}

func TestRoll_RollsInRange(t *testing.T) {
	rng := seededRand(42)
	for range 200 {
		r, err := Roll("3d6+1", rng)
		require.NoError(t, err)
		require.Len(t, r.Rolls, 3)
		for _, x := range r.Rolls {
			assert.GreaterOrEqual(t, x, 1)
			assert.LessOrEqual(t, x, 6)
		}
		assert.Equal(t, 1, r.Modifier)
		assert.GreaterOrEqual(t, r.Total, 4)
		assert.LessOrEqual(t, r.Total, 19)
	}
}

func TestRoll_NegativeTerm(t *testing.T) {
	rng := seededRand(7)
	r, err := Roll("2d6-2d4", rng)
	require.NoError(t, err)
	require.Len(t, r.Rolls, 4)
	// 前 2 个为正、后 2 个为负
	assert.Greater(t, r.Rolls[0], 0)
	assert.Greater(t, r.Rolls[1], 0)
	assert.Less(t, r.Rolls[2], 0)
	assert.Less(t, r.Rolls[3], 0)
}

func TestRoll_Invalid(t *testing.T) {
	bads := []string{
		"",
		"d6",
		"1d",
		"1d0",
		"0d6",
		"abc",
		"1d6+",
		"+",
		"1d6++2",
		"1d6 d6",
		"101d6",
		"1d101",
		"1d6*2",
	}
	for _, b := range bads {
		t.Run(b, func(t *testing.T) {
			_, err := Roll(b, seededRand(1))
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrInvalidDiceExpression), "expected ErrInvalidDiceExpression: got %v", err)
		})
	}
}

func TestRoll_LeadingSign(t *testing.T) {
	r, err := Roll("-3", seededRand(1))
	require.NoError(t, err)
	assert.Equal(t, -3, r.Total)

	r, err = Roll("+1d6", seededRand(1))
	require.NoError(t, err)
	require.Len(t, r.Rolls, 1)
	assert.GreaterOrEqual(t, r.Total, 1)
	assert.LessOrEqual(t, r.Total, 6)
}
