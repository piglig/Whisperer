package tools

import (
	"context"
	"encoding/json"

	"github.com/zhuzhenwu/whisperer/internal/store"
)

func init() {
	register("update_investigator_vitals", handleUpdateInvestigatorVitals)
	register("update_npc_relation", handleUpdateNPCRelation)
	register("kill_npc", handleKillNPC)
	register("mark_location_visited", handleMarkLocationVisited)
	register("move_item", handleMoveItem)
	register("destroy_item", handleDestroyItem)
	register("mark_clue_found", handleMarkClueFound)
	register("transition_location", handleTransitionLocation)
	register("add_event", handleAddEvent)
}

type updateVitalsIn struct {
	InvestigatorID string `json:"investigator_id"`
	HP             int    `json:"hp"`
	MP             int    `json:"mp"`
	SAN            int    `json:"san"`
}

func handleUpdateInvestigatorVitals(ctx context.Context, d *Dispatcher, raw json.RawMessage) (any, bool) {
	in, err := decode[updateVitalsIn](raw)
	if err != nil {
		return errPayload(err.Error()), true
	}
	if err := d.repo.UpdateInvestigatorVitals(ctx, in.InvestigatorID, in.HP, in.MP, in.SAN); err != nil {
		return errPayload(err.Error()), true
	}
	if in.HP <= 0 || in.SAN <= 0 {
		// 失败收束触发条件之一（SLA #8）；store 标 deactivate，orchestrator 上层负责进入结局页。
		if err := d.repo.DeactivateInvestigator(ctx, in.InvestigatorID); err != nil {
			return errPayload("deactivate investigator: " + err.Error()), true
		}
		markAutosave(ctx, d, "investigator_lost:"+in.InvestigatorID)
	}
	return map[string]any{"ok": true}, false
}

type updateRelationIn struct {
	NPCID string `json:"npc_id"`
	Delta int    `json:"delta"`
}

func handleUpdateNPCRelation(ctx context.Context, d *Dispatcher, raw json.RawMessage) (any, bool) {
	in, err := decode[updateRelationIn](raw)
	if err != nil {
		return errPayload(err.Error()), true
	}
	if err := d.repo.UpdateNPCRelation(ctx, in.NPCID, in.Delta); err != nil {
		return errPayload(err.Error()), true
	}
	return map[string]any{"ok": true}, false
}

func handleKillNPC(ctx context.Context, d *Dispatcher, raw json.RawMessage) (any, bool) {
	in, err := decode[npcIDIn](raw)
	if err != nil {
		return errPayload(err.Error()), true
	}
	if err := d.repo.KillNPC(ctx, in.NPCID); err != nil {
		return errPayload(err.Error()), true
	}
	markAutosave(ctx, d, "npc_killed:"+in.NPCID)
	return map[string]any{"ok": true}, false
}

func handleMarkLocationVisited(ctx context.Context, d *Dispatcher, raw json.RawMessage) (any, bool) {
	in, err := decode[locIDIn](raw)
	if err != nil {
		return errPayload(err.Error()), true
	}
	if err := d.repo.MarkLocationVisited(ctx, in.LocationID); err != nil {
		return errPayload(err.Error()), true
	}
	return map[string]any{"ok": true}, false
}

type moveItemIn struct {
	ItemID    string `json:"item_id"`
	OwnerType string `json:"owner_type"`
	OwnerID   string `json:"owner_id"`
}

func handleMoveItem(ctx context.Context, d *Dispatcher, raw json.RawMessage) (any, bool) {
	in, err := decode[moveItemIn](raw)
	if err != nil {
		return errPayload(err.Error()), true
	}
	owner := store.OwnerType(in.OwnerType)
	switch owner {
	case store.OwnerNPC, store.OwnerLocation, store.OwnerInvestigator, store.OwnerNone:
	default:
		return errPayload("owner_type must be one of: npc, location, investigator, none"), true
	}
	if err := d.repo.MoveItem(ctx, in.ItemID, owner, in.OwnerID); err != nil {
		return errPayload(err.Error()), true
	}
	return map[string]any{"ok": true}, false
}

type destroyItemIn struct {
	ItemID string `json:"item_id"`
}

func handleDestroyItem(ctx context.Context, d *Dispatcher, raw json.RawMessage) (any, bool) {
	in, err := decode[destroyItemIn](raw)
	if err != nil {
		return errPayload(err.Error()), true
	}
	if err := d.repo.DestroyItem(ctx, in.ItemID); err != nil {
		return errPayload(err.Error()), true
	}
	return map[string]any{"ok": true}, false
}

type markClueIn struct {
	ClueID     string `json:"clue_id"`
	LocationID string `json:"location_id"`
}

func handleMarkClueFound(ctx context.Context, d *Dispatcher, raw json.RawMessage) (any, bool) {
	in, err := decode[markClueIn](raw)
	if err != nil {
		return errPayload(err.Error()), true
	}
	if err := d.repo.MarkClueFound(ctx, in.ClueID, in.LocationID, d.turn); err != nil {
		return errPayload(err.Error()), true
	}
	// 写一条 narrative 事件，description 以 "clue_found:" 前缀打头，
	// 让 scenario.Detector 把本回合识别为主线推进（W7 e2e 修补：之前 mark_clue_found
	// 不计 progress 导致 drift 假警报）。
	related, _ := json.Marshal([]string{in.ClueID})
	_, _ = d.repo.AppendEvent(ctx, store.Event{
		SaveID:              d.saveID,
		Turn:                d.turn,
		Type:                store.EventNarrative,
		Description:         "clue_found:" + in.ClueID,
		RelatedEntitiesJSON: string(related),
	})
	return map[string]any{"ok": true}, false
}

type transitionIn struct {
	LocationID string `json:"location_id"`
}

func handleTransitionLocation(ctx context.Context, d *Dispatcher, raw json.RawMessage) (any, bool) {
	in, err := decode[transitionIn](raw)
	if err != nil {
		return errPayload(err.Error()), true
	}
	if err := d.repo.UpdateSaveProgress(ctx, d.saveID, in.LocationID, d.turn); err != nil {
		return errPayload(err.Error()), true
	}
	if err := d.repo.MarkLocationVisited(ctx, in.LocationID); err != nil {
		// 目的地可能尚未在表里（剧本由 W5 注入），失败时不视为致命错误，仅返回部分成功。
		return map[string]any{"ok": true, "warning": err.Error()}, false
	}
	markAutosave(ctx, d, "scene_transition:"+in.LocationID)
	return map[string]any{"ok": true}, false
}

type addEventIn struct {
	Type            string   `json:"type"`
	Description     string   `json:"description"`
	RelatedEntities []string `json:"related_entities"`
}

func handleAddEvent(ctx context.Context, d *Dispatcher, raw json.RawMessage) (any, bool) {
	in, err := decode[addEventIn](raw)
	if err != nil {
		return errPayload(err.Error()), true
	}
	related, _ := json.Marshal(in.RelatedEntities)
	id, err := d.repo.AppendEvent(ctx, store.Event{
		SaveID:              d.saveID,
		Turn:                d.turn,
		Type:                store.EventType(in.Type),
		Description:         in.Description,
		RelatedEntitiesJSON: string(related),
	})
	if err != nil {
		return errPayload(err.Error()), true
	}
	return map[string]any{"event_id": id}, false
}
