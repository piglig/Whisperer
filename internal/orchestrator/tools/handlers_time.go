package tools

import (
	"context"
	"encoding/json"

	"github.com/zhuzhenwu/whisperer/internal/store"
)

func init() {
	register("get_time_of_day", handleGetTimeOfDay)
	register("advance_time", handleAdvanceTime)
}

func handleGetTimeOfDay(ctx context.Context, d *Dispatcher, _ json.RawMessage) (any, bool) {
	sv, err := d.repo.GetSave(ctx, d.saveID)
	if err != nil {
		return errPayload(err.Error()), true
	}
	return map[string]any{"time_of_day": string(sv.TimeOfDay)}, false
}

type advanceTimeIn struct {
	Stages int `json:"stages"`
}

func handleAdvanceTime(ctx context.Context, d *Dispatcher, raw json.RawMessage) (any, bool) {
	in, err := decode[advanceTimeIn](raw)
	if err != nil {
		return errPayload(err.Error()), true
	}
	stages := in.Stages
	if stages <= 0 {
		stages = 1
	}
	sv, err := d.repo.GetSave(ctx, d.saveID)
	if err != nil {
		return errPayload(err.Error()), true
	}
	t := sv.TimeOfDay
	if t == "" {
		t = store.TimeMorning
	}
	for range stages {
		t = t.Next()
	}
	if err := d.repo.SetTimeOfDay(ctx, d.saveID, t); err != nil {
		return errPayload(err.Error()), true
	}
	return map[string]any{"time_of_day": string(t), "stages_advanced": stages}, false
}
