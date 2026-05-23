package scenario

import (
	"fmt"
	"sort"
	"strings"
)

// ClueLead 描述"本来可以走哪条路径拿到这条线索"。结局报告里展示给玩家，
// 让漏掉的线索能定位到具体 NPC / 地点 / 触发器，支撑跨周目复盘。
type ClueLead struct {
	// Kind: "trigger" | "location" | "npc"
	Kind string `json:"kind"`
	// Hint 是一句话级别的人类可读引导，例如 "去 教堂 + 让 卡尔文神父 好感 > 9"。
	Hint string `json:"hint"`
	// TriggerID / LocationID / NPCID 视 Kind 而定，可空。
	TriggerID  string `json:"trigger_id,omitempty"`
	LocationID string `json:"location_id,omitempty"`
	NPCID      string `json:"npc_id,omitempty"`
}

// LeadsForClue 静态分析 scenario，列出能拿到 clueID 的所有路径。
//
// 三类来源：
//   - trigger: 任何 Trigger.Then 里 mark_clue_found 指向该 clue 的触发器
//   - location: SClue.Location 显式声明的地点
//   - npc: SClue.Source 显式声明的 NPC（即"这条线索的持有者"）
//
// GM 通过 mark_clue_found tool 临时发放的线索无法静态预测——那不是 lead，
// 是 LLM 即兴；只能靠以上三类锚点。
//
// 返回的 leads 按 Kind 稳定排序（trigger → location → npc），便于断言。
func LeadsForClue(s *Scenario, clueID string) []ClueLead {
	if s == nil || clueID == "" {
		return nil
	}
	var leads []ClueLead

	for _, trigger := range s.Triggers {
		grants := false
		for _, action := range trigger.Then {
			if action.MarkClueFound != nil && action.MarkClueFound.ClueID == clueID {
				grants = true
				break
			}
		}
		if !grants {
			continue
		}
		hint := humanizeCondition(s, trigger.When)
		if hint == "" {
			hint = "触发器 " + trigger.ID
		} else {
			hint = "条件：" + hint
		}
		leads = append(leads, ClueLead{
			Kind:      "trigger",
			Hint:      hint,
			TriggerID: trigger.ID,
		})
	}

	for _, clue := range s.Clues {
		if clue.ID != clueID {
			continue
		}
		if clue.Location != "" {
			leads = append(leads, ClueLead{
				Kind:       "location",
				Hint:       "去 " + locationLabel(s, clue.Location),
				LocationID: clue.Location,
			})
		}
		if clue.Source != "" {
			leads = append(leads, ClueLead{
				Kind:  "npc",
				Hint:  "向 " + npcLabel(s, clue.Source) + " 追问",
				NPCID: clue.Source,
			})
		}
		break
	}

	sort.SliceStable(leads, func(i, j int) bool {
		return leadKindOrder(leads[i].Kind) < leadKindOrder(leads[j].Kind)
	})
	return leads
}

func leadKindOrder(kind string) int {
	switch kind {
	case "trigger":
		return 0
	case "location":
		return 1
	case "npc":
		return 2
	}
	return 9
}

// humanizeCondition 把 Condition 翻译成一句中文。空条件返回 ""。
//
// 翻译规则：
//   - all: 用 " + " 连接子项
//   - any: 用 " 或 " 连接子项
//   - not: 前缀 "非 "
//   - current_location/location_visited: 解析地点 ID → 名字
//   - npc_relation_gt/lt: 解析 NPC ID → 名字
//   - trigger_fired / clue_found: 显式 ID（trigger 没有名字；clue 走 description）
func humanizeCondition(s *Scenario, c Condition) string {
	if len(c.All) > 0 {
		parts := humanizeChildren(s, c.All)
		return strings.Join(parts, " + ")
	}
	if len(c.Any) > 0 {
		parts := humanizeChildren(s, c.Any)
		return strings.Join(parts, " 或 ")
	}
	if c.Not != nil {
		inner := humanizeCondition(s, *c.Not)
		if inner == "" {
			return ""
		}
		return "非（" + inner + "）"
	}
	switch {
	case c.CurrentLocation != "":
		return "在 " + locationLabel(s, c.CurrentLocation)
	case len(c.LocationIn) > 0:
		names := make([]string, 0, len(c.LocationIn))
		for _, id := range c.LocationIn {
			names = append(names, locationLabel(s, id))
		}
		return "在 " + strings.Join(names, " 或 ")
	case c.LocationVisited != "":
		return "曾去过 " + locationLabel(s, c.LocationVisited)
	case c.ClueFound != "":
		return "已发现：" + clueLabel(s, c.ClueFound)
	case c.NPCDead != "":
		return npcLabel(s, c.NPCDead) + " 已死亡"
	case c.NPCRelationGT != nil:
		return fmt.Sprintf("%s 好感 > %d", npcLabel(s, c.NPCRelationGT.NPC), c.NPCRelationGT.Value)
	case c.NPCRelationLT != nil:
		return fmt.Sprintf("%s 好感 < %d", npcLabel(s, c.NPCRelationLT.NPC), c.NPCRelationLT.Value)
	case c.TimeOfDay != "":
		return timeOfDayLabel(c.TimeOfDay)
	case c.TurnGE > 0:
		return fmt.Sprintf("第 %d 回合之后", c.TurnGE)
	case c.TriggerFired != "":
		return "已触发：" + c.TriggerFired
	}
	return ""
}

func humanizeChildren(s *Scenario, cs []Condition) []string {
	out := make([]string, 0, len(cs))
	for _, child := range cs {
		if h := humanizeCondition(s, child); h != "" {
			out = append(out, h)
		}
	}
	return out
}

func locationLabel(s *Scenario, id string) string {
	if s != nil {
		for _, loc := range s.Locations {
			if loc.ID == id && strings.TrimSpace(loc.Name) != "" {
				return loc.Name
			}
		}
	}
	return id
}

func npcLabel(s *Scenario, id string) string {
	if s != nil {
		for _, npc := range s.NPCs {
			if npc.ID == id && strings.TrimSpace(npc.Name) != "" {
				return npc.Name
			}
		}
	}
	return id
}

func clueLabel(s *Scenario, id string) string {
	if s != nil {
		for _, clue := range s.Clues {
			if clue.ID == id && strings.TrimSpace(clue.Description) != "" {
				return clue.Description
			}
		}
	}
	return id
}

func timeOfDayLabel(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "morning":
		return "清晨"
	case "afternoon":
		return "午后"
	case "night":
		return "夜晚"
	}
	return t
}
