package rules

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpposedRoll_Outcomes(t *testing.T) {
	seenActor, seenTarget := false, false
	for seed := uint64(1); seed <= 1000; seed++ {
		r := OpposedRoll("alice", 60, "bob", 40, seededRand(seed))
		assert.Contains(t, []string{"actor", "target", "tie"}, r.Winner)
		switch r.Winner {
		case "actor":
			seenActor = true
		case "target":
			seenTarget = true
		}
	}
	assert.True(t, seenActor)
	assert.True(t, seenTarget)
}

func TestOpposedRoll_TieByEqualSkill(t *testing.T) {
	// 同技能值时大量 tie
	tieCount := 0
	for seed := uint64(1); seed <= 1000; seed++ {
		r := OpposedRoll("a", 50, "b", 50, seededRand(seed))
		if r.Winner == "tie" {
			tieCount++
		}
	}
	assert.Greater(t, tieCount, 50, "equal skill should produce many ties")
}

func TestOpposedRoll_HigherDegreeWins(t *testing.T) {
	for seed := uint64(1); seed <= 200; seed++ {
		r := OpposedRoll("a", 70, "b", 30, seededRand(seed))
		if r.Actor.Degree != r.Target.Degree {
			expected := "actor"
			if degreeRank(r.Target.Degree) > degreeRank(r.Actor.Degree) {
				expected = "target"
			}
			assert.Equal(t, expected, r.Winner, "seed=%d actor=%s target=%s", seed, r.Actor.Degree, r.Target.Degree)
		}
	}
}
