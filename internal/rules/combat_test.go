package rules

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitiative_DEXOrder(t *testing.T) {
	cs := []Combatant{
		{ID: "slow", DEX: 30},
		{ID: "fast", DEX: 80},
		{ID: "mid", DEX: 50},
	}
	order := Initiative(cs, seededRand(1))
	assert.Equal(t, []string{"fast", "mid", "slow"}, order)
}

func TestInitiative_TieBreakerStable(t *testing.T) {
	cs := []Combatant{
		{ID: "a", DEX: 50},
		{ID: "b", DEX: 50},
		{ID: "c", DEX: 50},
	}
	// 同 seed 同输入应得到相同顺序（确定性）
	o1 := Initiative(cs, seededRand(7))
	o2 := Initiative(cs, seededRand(7))
	assert.Equal(t, o1, o2)
	assert.ElementsMatch(t, []string{"a", "b", "c"}, o1)
}

func TestAttack_HitOrMiss(t *testing.T) {
	att := Combatant{ID: "p", Name: "Player", DEX: 60, WeaponSkill: 50, WeaponDamage: "1d6", HP: 10}
	def := Combatant{ID: "n", Name: "Cultist", DEX: 40, WeaponSkill: 30, WeaponDamage: "1d4", HP: 8}

	hits, misses := 0, 0
	for seed := uint64(1); seed <= 500; seed++ {
		check, dmg, err := Attack(att, def, seededRand(seed))
		require.NoError(t, err)
		if check.Success {
			require.NotNil(t, dmg)
			assert.Greater(t, dmg.Total, 0)
			hits++
		} else {
			assert.Nil(t, dmg)
			misses++
		}
	}
	assert.Greater(t, hits, 0)
	assert.Greater(t, misses, 0)
}

func TestAttack_ImpalingExtremeMaxesDamage(t *testing.T) {
	att := Combatant{ID: "p", Name: "Player", WeaponSkill: 99, WeaponDamage: "1d6", Impaling: true}
	def := Combatant{ID: "n", Name: "Cultist"}

	plainMax, impaleMin := 0, 100
	for seed := uint64(1); seed <= 1000; seed++ {
		check, dmg, err := Attack(att, def, seededRand(seed))
		require.NoError(t, err)
		if !check.Success || dmg == nil {
			continue
		}
		switch check.Degree {
		case DegreeRegularSuccess, DegreeHardSuccess:
			if dmg.Total > plainMax {
				plainMax = dmg.Total
			}
		case DegreeExtremeSuccess, DegreeCriticalSuccess:
			if dmg.Total < impaleMin {
				impaleMin = dmg.Total
			}
		}
	}
	assert.GreaterOrEqual(t, impaleMin, 7, "impale on 1d6 should be ≥ 7")
	assert.LessOrEqual(t, plainMax, 6, "plain hit on 1d6 should be ≤ 6")
}

func TestAttack_NonImpalingExtremeNoBonus(t *testing.T) {
	// 非穿刺武器即使 extreme/critical 也按普通伤害；上限就是 1d6 的 6。
	att := Combatant{ID: "p", Name: "Player", WeaponSkill: 99, WeaponDamage: "1d6", Impaling: false}
	def := Combatant{ID: "n", Name: "Cultist"}
	for seed := uint64(1); seed <= 500; seed++ {
		check, dmg, err := Attack(att, def, seededRand(seed))
		require.NoError(t, err)
		if !check.Success || dmg == nil {
			continue
		}
		assert.LessOrEqual(t, dmg.Total, 6)
	}
}

func TestAttack_BadWeapon(t *testing.T) {
	att := Combatant{ID: "p", Name: "P", WeaponSkill: 99, WeaponDamage: "garbage"}
	def := Combatant{ID: "n", Name: "N"}
	_, _, err := Attack(att, def, seededRand(1))
	require.Error(t, err)
}
