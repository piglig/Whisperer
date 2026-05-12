// Package rules implements the deterministic CoC 7e rule engine.
//
// All exported functions are pure: they take values and an injectable
// *math/rand/v2.Rand, and return result structs. They do no IO, hold no
// global state, and do not depend on any sibling internal/* package.
package rules

// Difficulty 表示技能检定的请求难度。
type Difficulty string

const (
	DifficultyRegular Difficulty = "regular"
	DifficultyHard    Difficulty = "hard"
	DifficultyExtreme Difficulty = "extreme"
)

// Degree 表示一次检定的实际成功等级。
type Degree string

const (
	DegreeCriticalSuccess Degree = "critical_success"
	DegreeExtremeSuccess  Degree = "extreme_success"
	DegreeHardSuccess     Degree = "hard_success"
	DegreeRegularSuccess  Degree = "regular_success"
	DegreeFailure         Degree = "failure"
	DegreeFumble          Degree = "fumble"
)

// degreeRank 给出比较 Degree 大小的整数序，用于对抗检定。
func degreeRank(d Degree) int {
	switch d {
	case DegreeCriticalSuccess:
		return 4
	case DegreeExtremeSuccess:
		return 3
	case DegreeHardSuccess:
		return 2
	case DegreeRegularSuccess:
		return 1
	case DegreeFailure:
		return 0
	case DegreeFumble:
		return -1
	default:
		return -2
	}
}

// SkillCheckResult 是一次技能检定的完整 trace。
type SkillCheckResult struct {
	SkillName   string     `json:"skill_name"`
	SkillValue  int        `json:"skill_value"`
	Difficulty  Difficulty `json:"difficulty"`
	Roll        int        `json:"roll"`      // 1..100
	Threshold   int        `json:"threshold"` // 实际比对的阈值
	Degree      Degree     `json:"degree"`
	Success     bool       `json:"success"`
	BonusDice   int        `json:"bonus_dice,omitempty"`
	PenaltyDice int        `json:"penalty_dice,omitempty"`
	TensRolls   []int      `json:"tens_rolls,omitempty"` // 奖励/惩罚骰展开记录（十位骰候选）
	OnesRoll    int        `json:"ones_roll,omitempty"`  // 个位骰
}

// DamageResult 是一次骰子表达式求值的完整 trace。
type DamageResult struct {
	Expression string `json:"expression"`
	Rolls      []int  `json:"rolls"`
	Modifier   int    `json:"modifier"`
	Total      int    `json:"total"`
}

// SanityResult 是一次 SAN 检定的完整 trace。
type SanityResult struct {
	Roll                        int  `json:"roll"`
	Threshold                   int  `json:"threshold"`
	Success                     bool `json:"success"`
	Loss                        int  `json:"loss"`
	TriggeredTemporaryInsanity  bool `json:"triggered_temporary_insanity"`
	TriggeredIndefiniteInsanity bool `json:"triggered_indefinite_insanity"`
	NewSAN                      int  `json:"new_san"`
}

// OpposedResult 是一次对抗检定的结果。
type OpposedResult struct {
	Actor  SkillCheckResult `json:"actor"`
	Target SkillCheckResult `json:"target"`
	Winner string           `json:"winner"` // "actor" | "target" | "tie"
}

// Combatant 是战斗中的一个角色。
type Combatant struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	DEX          int    `json:"dex"`
	WeaponSkill  int    `json:"weapon_skill"`
	WeaponDamage string `json:"weapon_damage"` // 如 "1d6"
	HP           int    `json:"hp"`
	// Impaling 是否为穿刺类武器（刀/矛/箭/弩等）。仅穿刺武器在 extreme / critical
	// success 时触发 impale 规则（伤害骰最大化 + 加伤一次）。CoC 7e 规则原文。
	Impaling bool `json:"impaling,omitempty"`
}

// GrowthCheckResult 是一次成长检定的完整 trace。
type GrowthCheckResult struct {
	SkillName string `json:"skill_name"`
	OldValue  int    `json:"old_value"`
	Roll      int    `json:"roll"`
	Grew      bool   `json:"grew"`
	Increment int    `json:"increment"`
	NewValue  int    `json:"new_value"`
}
