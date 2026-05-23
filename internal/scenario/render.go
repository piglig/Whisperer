package scenario

import (
	"fmt"
	"sort"
	"strings"
)

// RenderTruth 返回剧本真相文本（已包含 variant patch 后的最终内容）。
// 空字符串表示该剧本未声明 truth；调用方据此决定是否在 GM prompt 中渲染对应段落。
func RenderTruth(s *Scenario) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(s.Truth)
}

// RenderOpeningBriefing returns the player-facing first screen narrative.
// It must not include implementation details such as scenario IDs or variants.
func RenderOpeningBriefing(s *Scenario) string {
	if s == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "《%s》\n\n", s.Title)
	if intro := strings.TrimSpace(s.Intro); intro != "" {
		b.WriteString(intro)
	} else {
		b.WriteString(fallbackOpeningIntro(s))
	}
	b.WriteString("\n\n")
	if objectives := openingObjectives(s); len(objectives) > 0 {
		b.WriteString("当前目标：\n")
		for _, objective := range objectives {
			fmt.Fprintf(&b, "- %s\n", objective)
		}
		b.WriteString("\n")
	}
	b.WriteString("可以从这些行动开始：\n")
	for _, action := range openingActions(s) {
		fmt.Fprintf(&b, "- %s\n", action)
	}
	b.WriteString("\n直接输入一句自然语言即可，例：\"我查看公告栏上的失踪启事\"。")
	return strings.TrimSpace(b.String())
}

func openingObjectives(s *Scenario) []string {
	if s == nil {
		return nil
	}
	if len(s.Objectives) == 0 {
		return nil
	}
	out := []string{}
	for _, obj := range s.Objectives {
		if strings.EqualFold(obj.Stage, "opening") || strings.EqualFold(obj.Stage, "act1") {
			out = append(out, obj.Title)
			for _, step := range obj.Steps {
				out = append(out, "  - "+step)
			}
			break
		}
	}
	return out
}

func fallbackOpeningIntro(s *Scenario) string {
	loc := startLocation(s)
	if loc.Name == "" {
		return "故事即将开始。你已经抵达事件现场，接下来要靠观察、询问与推理找出真相。"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "你抵达%s。", loc.Name)
	if loc.Description != "" {
		b.WriteString(loc.Description)
	}
	if names := startNPCNames(s); len(names) > 0 {
		fmt.Fprintf(&b, "\n\n附近可以接触的人：%s。", strings.Join(names, "、"))
	}
	return b.String()
}

func openingActions(s *Scenario) []string {
	loc := startLocation(s)
	actions := []string{}
	for _, lead := range loc.Leads {
		if strings.TrimSpace(lead) != "" {
			actions = append(actions, strings.TrimSpace(lead))
		}
	}
	if len(actions) == 0 && loc.Name != "" {
		actions = append(actions, "观察"+loc.Name+"，寻找异常痕迹或可调查的物件")
	}
	if npc := firstStartNPC(s); npc.Name != "" {
		if npc.OpeningLine != "" {
			actions = append(actions, fmt.Sprintf("与%s交谈，她也许会说：%s", npc.Name, strings.TrimSpace(npc.OpeningLine)))
		} else {
			actions = append(actions, "与"+npc.Name+"交谈，询问最近发生了什么")
		}
	}
	actions = appendUniqueStrings(actions,
		"沿着可见道路前往酒馆、巡警所、教堂或灯塔等地点",
		"查看自己的案件卡，确认当前地点、人物和已发现线索",
	)
	return actions
}

func firstStartNPC(s *Scenario) SNPC {
	if names := startNPCNames(s); len(names) > 0 {
		for _, npc := range s.NPCs {
			if npc.Name == names[0] {
				return npc
			}
		}
	}
	return SNPC{}
}

func appendUniqueStrings(values []string, additions ...string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additions {
		if value == "" || seen[value] {
			continue
		}
		values = append(values, value)
		seen[value] = true
	}
	return values
}

func startLocation(s *Scenario) SLocation {
	if s == nil {
		return SLocation{}
	}
	for _, loc := range s.Locations {
		if loc.ID == s.Start.Location {
			return loc
		}
	}
	return SLocation{}
}

func startNPCNames(s *Scenario) []string {
	if s == nil || s.Start.Location == "" {
		return nil
	}
	names := []string{}
	for _, npc := range s.NPCs {
		if npc.Location == s.Start.Location && npc.Name != "" {
			names = append(names, npc.Name)
		}
	}
	return names
}

// RenderPlayerGuidance returns player-facing scenario scaffolding for the GM.
func RenderPlayerGuidance(s *Scenario) string {
	if s == nil {
		return ""
	}
	var b strings.Builder
	if len(s.Objectives) > 0 {
		b.WriteString("## 目标\n")
		for _, obj := range s.Objectives {
			fmt.Fprintf(&b, "- %s：%s\n", obj.Stage, obj.Title)
			for _, step := range obj.Steps {
				fmt.Fprintf(&b, "  - %s\n", strings.TrimSpace(step))
			}
		}
	}
	if len(s.Locations) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("## 地点行动提示\n")
		for _, loc := range s.Locations {
			if len(loc.Leads) == 0 {
				continue
			}
			fmt.Fprintf(&b, "- %s\n", loc.Name)
			for _, lead := range loc.Leads {
				fmt.Fprintf(&b, "  - %s\n", strings.TrimSpace(lead))
			}
		}
	}
	if len(s.Threats) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("## 风险轨\n")
		for _, threat := range s.Threats {
			fmt.Fprintf(&b, "- %s：%s\n", threat.Name, strings.TrimSpace(threat.Description))
			for _, state := range threat.States {
				fmt.Fprintf(&b, "  - %s（%d）：%s\n", strings.TrimSpace(state.Label), state.Severity, strings.TrimSpace(state.Description))
			}
		}
	}
	if len(s.NPCs) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("## NPC 初见\n")
		for _, npc := range s.NPCs {
			if npc.FirstImpression == "" && npc.OpeningLine == "" {
				continue
			}
			fmt.Fprintf(&b, "- %s\n", npc.Name)
			if npc.FirstImpression != "" {
				fmt.Fprintf(&b, "  - 初见：%s\n", strings.TrimSpace(npc.FirstImpression))
			}
			if npc.OpeningLine != "" {
				fmt.Fprintf(&b, "  - 第一句：%s\n", strings.TrimSpace(npc.OpeningLine))
			}
			for _, opt := range npc.DialogueOptions {
				fmt.Fprintf(&b, "  - 可问：%s → %s\n", strings.TrimSpace(opt.Label), strings.TrimSpace(opt.Prompt))
			}
		}
	}
	if len(s.Items) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("## 物品行动\n")
		for _, item := range s.Items {
			if len(item.Actions) == 0 {
				continue
			}
			fmt.Fprintf(&b, "- %s\n", item.Name)
			for _, action := range item.Actions {
				fmt.Fprintf(&b, "  - %s → %s\n", strings.TrimSpace(action.Label), strings.TrimSpace(action.Prompt))
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func RenderThreatStatuses(statuses []ThreatStatus) string {
	if len(statuses) == 0 {
		return ""
	}
	var b strings.Builder
	for _, status := range statuses {
		fmt.Fprintf(&b, "- %s：%s（风险 %d）", status.Name, status.StateLabel, status.Severity)
		if status.StateDescription != "" {
			fmt.Fprintf(&b, " — %s", strings.TrimSpace(status.StateDescription))
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// RenderNPCSecrets 返回每位 NPC 的隐藏动机表（markdown）。
// 空字符串表示无 NPC 声明 secret。
func RenderNPCSecrets(s *Scenario) string {
	if s == nil {
		return ""
	}
	var b strings.Builder
	for _, n := range s.NPCs {
		sec := strings.TrimSpace(n.Secret)
		if sec == "" {
			continue
		}
		fmt.Fprintf(&b, "- **%s** (`%s`)：%s\n", n.Name, n.ID, sec)
	}
	return strings.TrimRight(b.String(), "\n")
}

// RenderNPCKnowledge 返回每位 NPC 的关键词解锁知识表（markdown 嵌套列表）。
// 空字符串表示无 NPC 声明 knowledge。
func RenderNPCKnowledge(s *Scenario) string {
	if s == nil {
		return ""
	}
	var b strings.Builder
	for _, n := range s.NPCs {
		if len(n.Knowledge) == 0 {
			continue
		}
		fmt.Fprintf(&b, "- **%s** (`%s`)\n", n.Name, n.ID)
		keys := make([]string, 0, len(n.Knowledge))
		for k := range n.Knowledge {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			kn := n.Knowledge[k]
			line := fmt.Sprintf("  - `%s`：", k)
			if len(kn.RequiresPhrases) > 0 {
				line += fmt.Sprintf("关键词 [%s] → ", strings.Join(kn.RequiresPhrases, " / "))
			}
			line += strings.TrimSpace(kn.Reveal)
			if kn.SanLoss != "" {
				line += fmt.Sprintf("（SAN 损失 %s）", kn.SanLoss)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// RenderNPCKnowledgeFor 返回单个 NPC 的关键词解锁知识表（用于 NPC 子代理 system prompt）。
// 找不到该 npcID 或无 knowledge 时返回空字符串。
func RenderNPCKnowledgeFor(s *Scenario, npcID string) string {
	if s == nil || npcID == "" {
		return ""
	}
	for _, n := range s.NPCs {
		if n.ID != npcID {
			continue
		}
		if len(n.Knowledge) == 0 {
			return ""
		}
		var b strings.Builder
		keys := make([]string, 0, len(n.Knowledge))
		for k := range n.Knowledge {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			kn := n.Knowledge[k]
			line := fmt.Sprintf("- `%s`：", k)
			if len(kn.RequiresPhrases) > 0 {
				line += fmt.Sprintf("仅在玩家提及 [%s] 时透露 → ", strings.Join(kn.RequiresPhrases, " / "))
			} else {
				line += "（无关键词门槛） → "
			}
			line += strings.TrimSpace(kn.Reveal)
			if kn.SanLoss != "" {
				line += fmt.Sprintf("（SAN 损失 %s）", kn.SanLoss)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
		return strings.TrimRight(b.String(), "\n")
	}
	return ""
}

// RenderClueAtlas 返回 tier 分级的线索网（markdown）。
// tier 1/2/3 → 三层主线；tier 0 或未声明 → 视为 red herring（单独成段）。
func RenderClueAtlas(s *Scenario) string {
	if s == nil || len(s.Clues) == 0 {
		return ""
	}
	tiers := map[int][]SClue{}
	var rh []SClue
	for _, c := range s.Clues {
		if c.Tier <= 0 {
			rh = append(rh, c)
			continue
		}
		tiers[c.Tier] = append(tiers[c.Tier], c)
	}
	var b strings.Builder
	keys := []int{1, 2, 3}
	labels := map[int]string{
		1: "Tier 1（表层）",
		2: "Tier 2（共谋层）",
		3: "Tier 3（神话层）",
	}
	for _, k := range keys {
		clues := tiers[k]
		if len(clues) == 0 {
			continue
		}
		fmt.Fprintf(&b, "## %s\n", labels[k])
		for _, c := range clues {
			renderClueLine(&b, c)
		}
		b.WriteString("\n")
	}
	if len(rh) > 0 {
		b.WriteString("## Red herring（误导，非主线）\n")
		for _, c := range rh {
			renderClueLine(&b, c)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderClueLine(b *strings.Builder, c SClue) {
	fmt.Fprintf(b, "- `%s`", c.ID)
	if c.Location != "" {
		fmt.Fprintf(b, "（位置 %s", c.Location)
		if c.Source != "" {
			fmt.Fprintf(b, "，来源 %s", c.Source)
		}
		b.WriteString("）")
	} else if c.Source != "" {
		fmt.Fprintf(b, "（来源 %s）", c.Source)
	}
	if c.SanLoss != "" {
		fmt.Fprintf(b, " · SAN 损失 %s", c.SanLoss)
	}
	if c.Description != "" {
		fmt.Fprintf(b, " — %s", strings.TrimSpace(c.Description))
	}
	b.WriteString("\n")
}
