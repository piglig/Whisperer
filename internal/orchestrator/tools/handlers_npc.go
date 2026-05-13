package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zhuzhenwu/whisperer/internal/agent"
	"github.com/zhuzhenwu/whisperer/internal/memory"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

func init() {
	register("npc_speak", handleNPCSpeak)
}

type npcSpeakIn struct {
	NPCID      string `json:"npc_id"`
	Intent     string `json:"intent"`
	PlayerLine string `json:"player_line"`
}

// handleNPCSpeak 桥接 GM ↔ NPC 子代理：
//  1. 从 store 读 NPC profile。
//  2. 从 memory 检索该 NPC 相关的近期事件（top-3）作为上下文。
//  3. 调 NPCAgent.Speak 生成单段台词。
//  4. 把台词写回 events（在 SQLite 与 memory 各一份），便于下回合检索。
//
// memory / npcAgent 任一未注入 → 返回 isError=true。
func handleNPCSpeak(ctx context.Context, d *Dispatcher, raw json.RawMessage) (any, bool) {
	if d.npcAgent == nil {
		return errPayload("npc_speak: no NPC agent configured"), true
	}
	in, err := decode[npcSpeakIn](raw)
	if err != nil {
		return errPayload(err.Error()), true
	}
	if strings.TrimSpace(in.NPCID) == "" || strings.TrimSpace(in.Intent) == "" {
		return errPayload("npc_speak: npc_id and intent are required"), true
	}
	npc, err := d.repo.GetNPC(ctx, in.NPCID)
	if err != nil {
		return errPayload("get npc: " + err.Error()), true
	}
	if !npc.Alive {
		return errPayload(fmt.Sprintf("npc_speak: %s is no longer alive", npc.Name)), true
	}

	persona := buildPersona(npc)

	var recent string
	if d.memory != nil {
		recent = retrieveNPCHistory(ctx, d.memory, npc, in.Intent)
	}

	var secret, knowledge string
	if d.scenario != nil {
		secret, knowledge = lookupSecretAndKnowledge(d.scenario, npc.ID)
	}

	dialogue, err := d.npcAgent.Speak(ctx, agent.NPCSpeakRequest{
		Persona:       persona,
		Secret:        secret,
		Knowledge:     knowledge,
		RecentHistory: recent,
		Intent:        in.Intent,
		PlayerLine:    in.PlayerLine,
	})
	if err != nil {
		return errPayload("npc agent: " + err.Error()), true
	}

	desc := fmt.Sprintf("%s: %s", npc.Name, dialogue)
	related, _ := json.Marshal([]string{npc.ID})
	eventID, err := d.repo.AppendEvent(ctx, store.Event{
		SaveID:              d.saveID,
		Turn:                d.turn,
		Type:                store.EventNarrative,
		Description:         desc,
		RelatedEntitiesJSON: string(related),
	})
	if err != nil {
		// 台词已生成但事件落库失败：仍把台词返回给 LLM，但标记错误。
		return map[string]any{
			"dialogue": dialogue,
			"warning":  "event persistence failed: " + err.Error(),
		}, true
	}
	if d.memory != nil {
		// 写入 memory 时失败不算致命；下回合检索缺这条历史而已。
		_ = d.memory.UpsertEvent(ctx, fmt.Sprintf("evt-%d", eventID), desc, map[string]string{
			"npc_id": npc.ID,
			"turn":   fmt.Sprintf("%d", d.turn),
			"type":   "npc_dialogue",
		})
	}

	return map[string]any{
		"dialogue": dialogue,
		"npc_id":   npc.ID,
		"event_id": eventID,
	}, false
}

// buildPersona 把 store.NPC 的零散字段拼成一段 NPC sub-prompt 用的人格档案。
func buildPersona(npc store.NPC) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Name: %s\n", npc.Name)
	if strings.TrimSpace(npc.Personality) != "" {
		fmt.Fprintf(&b, "Personality: %s\n", strings.TrimSpace(npc.Personality))
	}
	if strings.TrimSpace(npc.KnowledgeJSON) != "" && npc.KnowledgeJSON != "{}" {
		fmt.Fprintf(&b, "Knowledge (JSON): %s\n", npc.KnowledgeJSON)
	}
	fmt.Fprintf(&b, "Relation to player: %d\n", npc.RelationToPlayer)
	return b.String()
}

// lookupSecretAndKnowledge 从 effective scenario 找指定 NPC 的 secret 与 knowledge 渲染。
func lookupSecretAndKnowledge(s *scenario.Scenario, npcID string) (string, string) {
	for _, n := range s.NPCs {
		if n.ID == npcID {
			return strings.TrimSpace(n.Secret), scenario.RenderNPCKnowledgeFor(s, npcID)
		}
	}
	return "", ""
}

// retrieveNPCHistory 用 memory 拉 top-3 与 (NPC name + intent) 相关的事件，拼成段落。
//
// 失败时静默返回空字符串：检索缺失不应阻塞 NPC 说话。
func retrieveNPCHistory(ctx context.Context, m *memory.Memory, npc store.NPC, intent string) string {
	q := strings.TrimSpace(npc.Name + " " + intent)
	if q == "" {
		return ""
	}
	hits, err := m.QueryEvents(ctx, q, 3)
	if err != nil || len(hits) == 0 {
		return ""
	}
	var b strings.Builder
	for _, h := range hits {
		b.WriteString("- ")
		b.WriteString(h.Content)
		b.WriteString("\n")
	}
	return b.String()
}
