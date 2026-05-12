package tools

import (
	"context"
	"encoding/json"

	"github.com/zhuzhenwu/whisperer/internal/rules"
)

func init() {
	register("roll_skill", handleRollSkill)
	register("roll_damage", handleRollDamage)
	register("sanity_check", handleSanityCheck)
	register("opposed_roll", handleOpposedRoll)
}

type rollSkillIn struct {
	SkillName   string `json:"skill_name"`
	SkillValue  int    `json:"skill_value"`
	Difficulty  string `json:"difficulty"`
	BonusDice   int    `json:"bonus_dice"`
	PenaltyDice int    `json:"penalty_dice"`
}

func handleRollSkill(_ context.Context, d *Dispatcher, raw json.RawMessage) (any, bool) {
	in, err := decode[rollSkillIn](raw)
	if err != nil {
		return errPayload(err.Error()), true
	}
	diff := rules.Difficulty(in.Difficulty)
	switch diff {
	case rules.DifficultyRegular, rules.DifficultyHard, rules.DifficultyExtreme:
	default:
		return errPayload("difficulty must be one of: regular, hard, extreme"), true
	}
	res := rules.RollSkill(in.SkillName, in.SkillValue, diff, d.rng, in.BonusDice, in.PenaltyDice)
	return res, false
}

type rollDamageIn struct {
	Expression string `json:"expression"`
}

func handleRollDamage(_ context.Context, d *Dispatcher, raw json.RawMessage) (any, bool) {
	in, err := decode[rollDamageIn](raw)
	if err != nil {
		return errPayload(err.Error()), true
	}
	res, err := rules.Roll(in.Expression, d.rng)
	if err != nil {
		return errPayload(err.Error()), true
	}
	return res, false
}

type sanityCheckIn struct {
	LossPass string `json:"loss_pass"`
	LossFail string `json:"loss_fail"`
}

// handleSanityCheck 把 rules.SanityCheck 与 store 写入合成一次原子操作：
// 读 active investigator → 检定 → 写新 SAN（必要时 deactivate）→ 返回 trace。
func handleSanityCheck(ctx context.Context, d *Dispatcher, raw json.RawMessage) (any, bool) {
	in, err := decode[sanityCheckIn](raw)
	if err != nil {
		return errPayload(err.Error()), true
	}
	inv, err := d.repo.GetActiveInvestigator(ctx, d.saveID)
	if err != nil {
		return errPayload("get active investigator: " + err.Error()), true
	}
	res, err := rules.SanityCheck(inv.SAN, in.LossPass, in.LossFail, d.rng)
	if err != nil {
		return errPayload(err.Error()), true
	}
	if err := d.repo.UpdateInvestigatorVitals(ctx, inv.ID, inv.HP, inv.MP, res.NewSAN); err != nil {
		return errPayload("persist SAN: " + err.Error()), true
	}
	if res.TriggeredIndefiniteInsanity {
		markAutosave(ctx, d, "indefinite_insanity:"+inv.ID)
	}
	if res.NewSAN == 0 {
		// SAN 归零 → 不定性疯狂 + 退役（SLA #8 失败收束阻断）
		if err := d.repo.DeactivateInvestigator(ctx, inv.ID); err != nil {
			return errPayload("deactivate investigator: " + err.Error()), true
		}
		markAutosave(ctx, d, "san_zero:"+inv.ID)
	}
	return res, false
}

type opposedIn struct {
	ActorName   string `json:"actor_name"`
	ActorSkill  int    `json:"actor_skill"`
	TargetName  string `json:"target_name"`
	TargetSkill int    `json:"target_skill"`
}

func handleOpposedRoll(_ context.Context, d *Dispatcher, raw json.RawMessage) (any, bool) {
	in, err := decode[opposedIn](raw)
	if err != nil {
		return errPayload(err.Error()), true
	}
	res := rules.OpposedRoll(in.ActorName, in.ActorSkill, in.TargetName, in.TargetSkill, d.rng)
	return res, false
}
