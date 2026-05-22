package scenario

import (
	"errors"
	"fmt"
	"math/rand/v2"
)

// SelectVariant 按 weight 加权随机选一个 variant，把 patch merge 到 base，返回 effective
// scenario 与所选 variant id。base 不变；调用方只持有返回值。
//
// 如果 base 没有 variants，返回 base 的拷贝 + 空 id（也即"无 variant 模式"）。
// rng 为 nil 时使用全局默认源（不可复现），调用方需要确定性时务必传入 *rand.Rand。
func SelectVariant(base *Scenario, rng *rand.Rand) (*Scenario, string, error) {
	if base == nil {
		return nil, "", errors.New("scenario: base is nil")
	}
	if len(base.Variants) == 0 {
		clone := cloneScenario(base)
		return clone, "", nil
	}
	total := 0
	for _, v := range base.Variants {
		w := v.Weight
		if w <= 0 {
			w = 1
		}
		total += w
	}
	var pick int
	if rng != nil {
		pick = rng.IntN(total)
	} else {
		pick = rand.IntN(total)
	}
	cum := 0
	for i := range base.Variants {
		w := base.Variants[i].Weight
		if w <= 0 {
			w = 1
		}
		cum += w
		if pick < cum {
			eff := MergeVariant(base, &base.Variants[i])
			return eff, base.Variants[i].ID, nil
		}
	}
	return nil, "", fmt.Errorf("scenario: variant selection fell through (total=%d pick=%d)", total, pick)
}

// SelectVariantByID 强制选指定 id 的 variant 并 merge。CLI --variant 用这条路径。
// 如果 id 为空，等价于 SelectVariant；如果 id 不存在，返回错误（不静默回退）。
func SelectVariantByID(base *Scenario, id string) (*Scenario, string, error) {
	if base == nil {
		return nil, "", errors.New("scenario: base is nil")
	}
	if id == "" {
		clone := cloneScenario(base)
		return clone, "", nil
	}
	for i := range base.Variants {
		if base.Variants[i].ID == id {
			eff := MergeVariant(base, &base.Variants[i])
			return eff, id, nil
		}
	}
	return nil, "", fmt.Errorf("scenario: variant %q not found", id)
}

// MergeVariant 把 variant 的 patch 应用到 base，返回新 *Scenario。纯函数，不修改 base。
//
// patch 语义：
//   - Truth：若 variant.Truth 非空，覆盖 base.Truth
//   - NPCSecrets：替换对应 NPC 的 Secret
//   - NPCKnowledgeOverrides：合并到对应 NPC 的 Knowledge map（同 key 替换）
//   - ClueOverrides：CluePatch 的非空字段替换 base clue 对应字段
//   - TriggerOverrides：整体替换 trigger.When
//   - EndingDescOverrides：替换 ending.Description
func MergeVariant(base *Scenario, v *Variant) *Scenario {
	out := cloneScenario(base)
	if v == nil {
		return out
	}
	if v.Truth != "" {
		out.Truth = v.Truth
	}
	if v.Culprit != "" {
		out.Culprit = v.Culprit
	}
	for i := range out.NPCs {
		n := &out.NPCs[i]
		if sec, ok := v.NPCSecrets[n.ID]; ok {
			n.Secret = sec
		}
		if patch, ok := v.NPCKnowledgeOverrides[n.ID]; ok {
			if n.Knowledge == nil {
				n.Knowledge = map[string]NPCKnowledge{}
			}
			for k, kn := range patch {
				n.Knowledge[k] = kn
			}
		}
	}
	for i := range out.Clues {
		c := &out.Clues[i]
		if patch, ok := v.ClueOverrides[c.ID]; ok {
			if patch.Description != "" {
				c.Description = patch.Description
			}
			if patch.Location != "" {
				c.Location = patch.Location
			}
			if patch.Source != "" {
				c.Source = patch.Source
			}
			if patch.SanLoss != "" {
				c.SanLoss = patch.SanLoss
			}
		}
	}
	for i := range out.Triggers {
		t := &out.Triggers[i]
		if patch, ok := v.TriggerOverrides[t.ID]; ok {
			t.When = patch.When
		}
	}
	for i := range out.Endings {
		e := &out.Endings[i]
		if desc, ok := v.EndingDescOverrides[e.ID]; ok {
			e.Description = desc
		}
	}
	// 合并后清空 variants 字段——effective scenario 不再持有 variant 列表，避免后续误调用。
	out.Variants = nil
	return out
}

// cloneScenario 是 Scenario 的深拷贝（足够支持 MergeVariant 后的隔离）。
// slice/map 都会被新建；指针字段（Condition.Not 等）被深拷贝。
func cloneScenario(s *Scenario) *Scenario {
	if s == nil {
		return nil
	}
	out := *s
	out.Locations = append([]SLocation(nil), s.Locations...)
	for i, l := range s.Locations {
		out.Locations[i].Leads = append([]string(nil), l.Leads...)
		out.Locations[i].Connections = append([]string(nil), l.Connections...)
	}
	out.NPCs = make([]SNPC, len(s.NPCs))
	for i, n := range s.NPCs {
		out.NPCs[i] = n
		out.NPCs[i].DialogueOptions = make([]DialogueOption, len(n.DialogueOptions))
		for j, opt := range n.DialogueOptions {
			out.NPCs[i].DialogueOptions[j] = opt
			out.NPCs[i].DialogueOptions[j].Stages = append([]string(nil), opt.Stages...)
			out.NPCs[i].DialogueOptions[j].RequiresClues = append([]string(nil), opt.RequiresClues...)
			out.NPCs[i].DialogueOptions[j].SuppressIfClues = append([]string(nil), opt.SuppressIfClues...)
		}
		if n.Knowledge != nil {
			km := make(map[string]NPCKnowledge, len(n.Knowledge))
			for k, kn := range n.Knowledge {
				cp := kn
				cp.RequiresPhrases = append([]string(nil), kn.RequiresPhrases...)
				km[k] = cp
			}
			out.NPCs[i].Knowledge = km
		}
	}
	out.Clues = append([]SClue(nil), s.Clues...)
	out.Items = make([]SItem, len(s.Items))
	for i, item := range s.Items {
		out.Items[i] = item
		out.Items[i].Actions = make([]ItemAction, len(item.Actions))
		for j, action := range item.Actions {
			out.Items[i].Actions[j] = action
			out.Items[i].Actions[j].Stages = append([]string(nil), action.Stages...)
			out.Items[i].Actions[j].Locations = append([]string(nil), action.Locations...)
			out.Items[i].Actions[j].OwnerTypes = append([]string(nil), action.OwnerTypes...)
			out.Items[i].Actions[j].RequiresClues = append([]string(nil), action.RequiresClues...)
			out.Items[i].Actions[j].SuppressIfClues = append([]string(nil), action.SuppressIfClues...)
		}
	}
	out.KeyClues = append([]string(nil), s.KeyClues...)
	out.Triggers = make([]Trigger, len(s.Triggers))
	for i, t := range s.Triggers {
		out.Triggers[i] = t
		out.Triggers[i].When = cloneCondition(t.When)
		out.Triggers[i].Then = append([]Action(nil), t.Then...)
	}
	out.Endings = make([]Ending, len(s.Endings))
	for i, e := range s.Endings {
		out.Endings[i] = e
		out.Endings[i].When = cloneCondition(e.When)
	}
	out.Variants = append([]Variant(nil), s.Variants...)
	return &out
}

func cloneCondition(c Condition) Condition {
	out := c
	if len(c.All) > 0 {
		out.All = make([]Condition, len(c.All))
		for i, sub := range c.All {
			out.All[i] = cloneCondition(sub)
		}
	}
	if len(c.Any) > 0 {
		out.Any = make([]Condition, len(c.Any))
		for i, sub := range c.Any {
			out.Any[i] = cloneCondition(sub)
		}
	}
	if c.Not != nil {
		nc := cloneCondition(*c.Not)
		out.Not = &nc
	}
	if c.NPCRelationLT != nil {
		cp := *c.NPCRelationLT
		out.NPCRelationLT = &cp
	}
	if c.NPCRelationGT != nil {
		cp := *c.NPCRelationGT
		out.NPCRelationGT = &cp
	}
	return out
}
