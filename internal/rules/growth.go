package rules

import (
	"math/rand/v2"
	"sort"
)

// MaxSkillValue 是 CoC 7e 技能上限。≥ 此值不再触发成长。
const MaxSkillValue = 99

// GrowthCheck 执行单技能成长检定。
//
// 规则：
//   - oldValue ≥ MaxSkillValue → 不成长（roll 仍记录）。
//   - 否则掷 d100；roll > oldValue → 成长 1d10，封顶 MaxSkillValue。
func GrowthCheck(skillName string, oldValue int, rng *rand.Rand) GrowthCheckResult {
	roll := rng.IntN(100) + 1
	res := GrowthCheckResult{
		SkillName: skillName,
		OldValue:  oldValue,
		Roll:      roll,
		NewValue:  oldValue,
	}
	if oldValue >= MaxSkillValue {
		return res
	}
	if roll <= oldValue {
		return res
	}
	inc := rng.IntN(10) + 1 // 1d10
	res.Grew = true
	res.Increment = inc
	res.NewValue = min(MaxSkillValue, oldValue+inc)
	return res
}

// SettleGrowth 在剧本结束时为本局所有"成功使用过"的技能各做一次成长检定。
//
// 输入是 skillName -> currentValue，输出按 skillName 字典序排序，确保确定性。
func SettleGrowth(used map[string]int, rng *rand.Rand) []GrowthCheckResult {
	names := make([]string, 0, len(used))
	for k := range used {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]GrowthCheckResult, 0, len(names))
	for _, name := range names {
		out = append(out, GrowthCheck(name, used[name], rng))
	}
	return out
}
