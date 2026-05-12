package rules

import (
	"math/rand/v2"
)

// IndefiniteInsanityThreshold 是单次 SAN 损失触发不定性疯狂的阈值。
const IndefiniteInsanityThreshold = 5

// SanityCheck 执行 CoC 7e SAN 检定。
//
// 规则：
//   - roll d100 vs currentSAN，roll <= currentSAN 视为成功。
//   - 成功扣 lossPass，失败扣 lossFail；两者皆为骰子表达式（如 "0"、"1"、"1d4"、"1d10"）。
//   - 单次损失 ≥ IndefiniteInsanityThreshold → 触发不定性疯狂。
//   - 临时疯狂（单次损失 ≥ INT/5 但 < 5）由 orchestrator 携带调查员 INT 二次判定，
//     此处 TriggeredTemporaryInsanity 始终为 false（保持 rules 接口窄）。
//   - SAN 不为负，下限 0；上限由调用方依据 Cthulhu Mythos 限制（rules 不维护）。
//
// 解析 lossPass / lossFail 失败时返回 error。
func SanityCheck(
	currentSAN int,
	lossPass string,
	lossFail string,
	rng *rand.Rand,
) (SanityResult, error) {
	if currentSAN < 0 {
		currentSAN = 0
	}

	roll := rng.IntN(100) + 1 // 1..100
	success := roll <= currentSAN

	expr := lossFail
	if success {
		expr = lossPass
	}
	dmg, err := Roll(expr, rng)
	if err != nil {
		return SanityResult{}, err
	}
	loss := max(0, dmg.Total)
	newSAN := max(0, currentSAN-loss)

	return SanityResult{
		Roll:                        roll,
		Threshold:                   currentSAN,
		Success:                     success,
		Loss:                        loss,
		TriggeredTemporaryInsanity:  false,
		TriggeredIndefiniteInsanity: loss >= IndefiniteInsanityThreshold,
		NewSAN:                      newSAN,
	}, nil
}
