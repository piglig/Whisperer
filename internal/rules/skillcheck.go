package rules

import "math/rand/v2"

// MaxBonusPenaltyDice 限制 bonus/penalty 骰数量。CoC 7e 规则文本里通常 ≤ 2，
// 但允许更高以适应未来扩展，封顶 4 防御性。
const MaxBonusPenaltyDice = 4

// RollSkill 执行 CoC 7e 技能检定。
//
// 参数:
//   - skillName: 仅用于 trace。
//   - skillValue: 0..99；超出范围按夹紧（防御性，不返回错误以保持调用方简单）。
//   - difficulty: regular/hard/extreme，决定阈值与"成功"判定。
//   - rng: 随机源。
//   - bonusDice / penaltyDice: ∈ [0, MaxBonusPenaltyDice]；同时给非零 → 抵消。
//
// 算法见 specs/01-rules-engine.md。
func RollSkill(
	skillName string,
	skillValue int,
	difficulty Difficulty,
	rng *rand.Rand,
	bonusDice int,
	penaltyDice int,
) SkillCheckResult {
	if skillValue < 0 {
		skillValue = 0
	}
	if skillValue > 99 {
		skillValue = 99
	}
	if bonusDice < 0 {
		bonusDice = 0
	}
	if penaltyDice < 0 {
		penaltyDice = 0
	}
	if bonusDice > MaxBonusPenaltyDice {
		bonusDice = MaxBonusPenaltyDice
	}
	if penaltyDice > MaxBonusPenaltyDice {
		penaltyDice = MaxBonusPenaltyDice
	}

	// CoC 7e 规则：奖励/惩罚同时给 → 抵消，保留差额方向。
	netBonus, netPenalty := bonusDice, penaltyDice
	if netBonus > 0 && netPenalty > 0 {
		switch {
		case netBonus > netPenalty:
			netBonus -= netPenalty
			netPenalty = 0
		case netPenalty > netBonus:
			netPenalty -= netBonus
			netBonus = 0
		default:
			netBonus, netPenalty = 0, 0
		}
	}

	roll, tens, ones := resolveBonusPenalty(rng, netBonus, netPenalty)

	threshold := skillValue
	switch difficulty {
	case DifficultyHard:
		threshold = skillValue / 2
	case DifficultyExtreme:
		threshold = skillValue / 5
	}

	degree := classifyDegree(roll, skillValue)

	success := false
	switch difficulty {
	case DifficultyRegular:
		success = degree == DegreeCriticalSuccess ||
			degree == DegreeExtremeSuccess ||
			degree == DegreeHardSuccess ||
			degree == DegreeRegularSuccess
	case DifficultyHard:
		success = degree == DegreeCriticalSuccess ||
			degree == DegreeExtremeSuccess ||
			degree == DegreeHardSuccess
	case DifficultyExtreme:
		success = degree == DegreeCriticalSuccess ||
			degree == DegreeExtremeSuccess
	}

	return SkillCheckResult{
		SkillName:   skillName,
		SkillValue:  skillValue,
		Difficulty:  difficulty,
		Roll:        roll,
		Threshold:   threshold,
		Degree:      degree,
		Success:     success,
		BonusDice:   bonusDice,
		PenaltyDice: penaltyDice,
		TensRolls:   tens,
		OnesRoll:    ones,
	}
}

// classifyDegree 仅根据 roll 与 skillValue 判定原始 degree（不考虑请求难度）。
func classifyDegree(roll, skillValue int) Degree {
	switch {
	case roll == 1:
		return DegreeCriticalSuccess
	case skillValue < 50 && roll >= 96:
		return DegreeFumble
	case skillValue >= 50 && roll == 100:
		return DegreeFumble
	case roll <= skillValue/5:
		return DegreeExtremeSuccess
	case roll <= skillValue/2:
		return DegreeHardSuccess
	case roll <= skillValue:
		return DegreeRegularSuccess
	default:
		return DegreeFailure
	}
}

// resolveBonusPenalty 实现 CoC 7e 奖励/惩罚骰：
//
//   - 个位 d10 掷一次（0..9）。
//   - 十位 d10 掷 1 + max(bonus, penalty) 次（每次 0..9）。
//   - bonus 取最低十位；penalty 取最高十位；两者均 0 时取首掷。
//   - 个位与十位均 0 视为 100。
func resolveBonusPenalty(rng *rand.Rand, bonus, penalty int) (roll int, tens []int, ones int) {
	ones = rng.IntN(10) // 0..9
	count := 1 + max(bonus, penalty)
	tens = make([]int, count)
	for i := range count {
		tens[i] = rng.IntN(10) // 0..9，分别表示 0/10/20/.../90
	}

	chosen := tens[0]
	switch {
	case bonus > 0 && penalty == 0:
		chosen = tens[0]
		for _, t := range tens[1:] {
			if t < chosen {
				chosen = t
			}
		}
	case penalty > 0 && bonus == 0:
		chosen = tens[0]
		for _, t := range tens[1:] {
			if t > chosen {
				chosen = t
			}
		}
	}

	roll = chosen*10 + ones
	if roll == 0 {
		roll = 100
	}
	return roll, tens, ones
}
