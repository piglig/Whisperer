package rules

import "math/rand/v2"

// OpposedRoll 执行对抗检定：双方各自掷常规检定，比较 degree → skillValue → tie。
func OpposedRoll(
	actorName string, actorValue int,
	targetName string, targetValue int,
	rng *rand.Rand,
) OpposedResult {
	actor := RollSkill(actorName, actorValue, DifficultyRegular, rng, 0, 0)
	target := RollSkill(targetName, targetValue, DifficultyRegular, rng, 0, 0)

	winner := "tie"
	switch {
	case degreeRank(actor.Degree) > degreeRank(target.Degree):
		winner = "actor"
	case degreeRank(target.Degree) > degreeRank(actor.Degree):
		winner = "target"
	default:
		switch {
		case actor.SkillValue > target.SkillValue:
			winner = "actor"
		case target.SkillValue > actor.SkillValue:
			winner = "target"
		}
	}

	return OpposedResult{Actor: actor, Target: target, Winner: winner}
}
