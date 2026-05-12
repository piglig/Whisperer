package tools

import (
	"context"
	"encoding/json"
)

func init() {
	register("get_investigator", handleGetInvestigator)
	register("get_npc", handleGetNPC)
	register("get_location", handleGetLocation)
	register("list_npcs_at_location", handleListNPCsAtLocation)
	register("list_found_clues", handleListFoundClues)
}

func handleGetInvestigator(ctx context.Context, d *Dispatcher, _ json.RawMessage) (any, bool) {
	inv, err := d.repo.GetActiveInvestigator(ctx, d.saveID)
	if err != nil {
		return errPayload(err.Error()), true
	}
	return inv, false
}

type npcIDIn struct {
	NPCID string `json:"npc_id"`
}

func handleGetNPC(ctx context.Context, d *Dispatcher, raw json.RawMessage) (any, bool) {
	in, err := decode[npcIDIn](raw)
	if err != nil {
		return errPayload(err.Error()), true
	}
	npc, err := d.repo.GetNPC(ctx, in.NPCID)
	if err != nil {
		return errPayload(err.Error()), true
	}
	return npc, false
}

type locIDIn struct {
	LocationID string `json:"location_id"`
}

func handleGetLocation(ctx context.Context, d *Dispatcher, raw json.RawMessage) (any, bool) {
	in, err := decode[locIDIn](raw)
	if err != nil {
		return errPayload(err.Error()), true
	}
	loc, err := d.repo.GetLocation(ctx, in.LocationID)
	if err != nil {
		return errPayload(err.Error()), true
	}
	return loc, false
}

func handleListNPCsAtLocation(ctx context.Context, d *Dispatcher, raw json.RawMessage) (any, bool) {
	in, err := decode[locIDIn](raw)
	if err != nil {
		return errPayload(err.Error()), true
	}
	npcs, err := d.repo.ListNPCsAtLocation(ctx, d.saveID, in.LocationID)
	if err != nil {
		return errPayload(err.Error()), true
	}
	return npcs, false
}

func handleListFoundClues(ctx context.Context, d *Dispatcher, _ json.RawMessage) (any, bool) {
	clues, err := d.repo.ListFoundClues(ctx, d.saveID)
	if err != nil {
		return errPayload(err.Error()), true
	}
	return clues, false
}
