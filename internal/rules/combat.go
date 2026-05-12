package rules

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"sort"
)

// ErrCombatInvalidWeapon 在武器伤害表达式无效时返回。
var ErrCombatInvalidWeapon = errors.New("invalid weapon damage expression")

// Initiative 返回战斗中行动顺序的 Combatant.ID 列表。
//
// 顺序：DEX 降序；同 DEX 各掷 d100，高者先（再相同则按输入顺序稳定）。
func Initiative(combatants []Combatant, rng *rand.Rand) []string {
	type entry struct {
		id    string
		dex   int
		tie   int
		index int
	}
	xs := make([]entry, len(combatants))
	for i, c := range combatants {
		xs[i] = entry{id: c.ID, dex: c.DEX, index: i, tie: rng.IntN(100) + 1}
	}
	sort.SliceStable(xs, func(i, j int) bool {
		if xs[i].dex != xs[j].dex {
			return xs[i].dex > xs[j].dex
		}
		if xs[i].tie != xs[j].tie {
			return xs[i].tie > xs[j].tie
		}
		return xs[i].index < xs[j].index
	})
	out := make([]string, len(xs))
	for i, e := range xs {
		out[i] = e.id
	}
	return out
}

// Attack 执行单次攻击：
//
//  1. 用 attacker.WeaponSkill 做常规技能检定（命名 = "attack:<weapon>"，trace 友好）。
//  2. 命中（regular_success 及以上）→ Roll(attacker.WeaponDamage)。
//  3. 穿刺武器（attacker.Impaling = true）+ extreme/critical →
//     impale 规则（伤害骰最大值 + 再掷一次表达式作为加伤）；
//     非穿刺武器即使 critical 也按普通伤害计算。
//
// 未命中返回 (check, nil, nil)。武器伤害表达式无效返回 (check, nil, error)。
func Attack(attacker, defender Combatant, rng *rand.Rand) (SkillCheckResult, *DamageResult, error) {
	check := RollSkill(
		fmt.Sprintf("attack:%s", attacker.Name),
		attacker.WeaponSkill,
		DifficultyRegular,
		rng,
		0, 0,
	)
	if !check.Success {
		return check, nil, nil
	}

	dmg, err := Roll(attacker.WeaponDamage, rng)
	if err != nil {
		return check, nil, fmt.Errorf("%w: %v", ErrCombatInvalidWeapon, err)
	}

	if attacker.Impaling &&
		(check.Degree == DegreeExtremeSuccess || check.Degree == DegreeCriticalSuccess) {
		dmg = applyImpale(dmg, attacker.WeaponDamage, rng)
	}
	_ = defender // 护甲/闪避/格挡留待后续 milestone。

	return check, &dmg, nil
}

// applyImpale 实现 CoC 7e 标准 impale 规则：
//   - 把 dmg.Rolls 中的每个骰值替换为该骰的最大面（依据原表达式重解析得到 sides）。
//   - 再掷一次表达式，加到 Total/Modifier/Rolls 中。
//
// 出错时（例如表达式异常）静默返回原 dmg；调用方此时已经验证过表达式可解析。
func applyImpale(dmg DamageResult, expr string, rng *rand.Rand) DamageResult {
	// 重新求最大面：根据表达式的 dice term 推导每骰的最大值。
	sides := inferDiceSides(expr)
	if sides == nil || len(sides) != len(dmg.Rolls) {
		// 无法对齐 → 退化：再掷一次表达式加上去即可（保留确定性）。
		extra, err := Roll(expr, rng)
		if err != nil {
			return dmg
		}
		dmg.Rolls = append(dmg.Rolls, extra.Rolls...)
		dmg.Modifier += extra.Modifier
		dmg.Total += extra.Total
		return dmg
	}

	for i, r := range dmg.Rolls {
		// dmg.Rolls 中的负数代表负向 term；取绝对值再换回符号。
		sign := 1
		if r < 0 {
			sign = -1
		}
		newVal := sign * sides[i]
		dmg.Total += (newVal - r)
		dmg.Rolls[i] = newVal
	}

	extra, err := Roll(expr, rng)
	if err != nil {
		return dmg
	}
	dmg.Rolls = append(dmg.Rolls, extra.Rolls...)
	dmg.Modifier += extra.Modifier
	dmg.Total += extra.Total
	return dmg
}

// inferDiceSides 从表达式中按 dice term 出现顺序提取每骰的面数序列。
// 仅支持 dice.go 定义的语法；纯整数 term 不贡献条目。
func inferDiceSides(expr string) []int {
	tokens, err := tokenize(expr)
	if err != nil {
		return nil
	}
	var sides []int
	for _, t := range tokens {
		if t == "+" || t == "-" {
			continue
		}
		idx := -1
		for i, r := range t {
			if r == 'd' {
				idx = i
				break
			}
		}
		if idx < 0 {
			continue
		}
		count, errC := atoiSafe(t[:idx])
		side, errS := atoiSafe(t[idx+1:])
		if errC != nil || errS != nil {
			return nil
		}
		for range count {
			sides = append(sides, side)
		}
	}
	return sides
}

func atoiSafe(s string) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not a digit: %q", r)
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}
