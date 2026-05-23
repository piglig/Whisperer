package scenario

import (
	"context"
	"fmt"
)

type ThreatStatus struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	StateID          string `json:"state_id"`
	StateLabel       string `json:"state_label"`
	Severity         int    `json:"severity"`
	StateDescription string `json:"state_description,omitempty"`
}

func (e *Engine) Threats(ctx context.Context, saveID string) ([]ThreatStatus, error) {
	if e.scenario == nil || len(e.scenario.Threats) == 0 {
		return nil, nil
	}
	fired, err := e.firedSet(ctx, saveID)
	if err != nil {
		return nil, err
	}
	view, err := e.snapshot(ctx, saveID, fired)
	if err != nil {
		return nil, err
	}
	statuses, err := EvaluateThreats(e.scenario, view)
	if err != nil {
		return nil, err
	}
	return statuses, nil
}

func EvaluateThreats(s *Scenario, view stateView) ([]ThreatStatus, error) {
	if s == nil || len(s.Threats) == 0 {
		return nil, nil
	}
	out := make([]ThreatStatus, 0, len(s.Threats))
	for _, threat := range s.Threats {
		state, err := matchingThreatState(threat, view)
		if err != nil {
			return nil, fmt.Errorf("threat %s: %w", threat.ID, err)
		}
		if state.ID == "" {
			continue
		}
		out = append(out, ThreatStatus{
			ID:               threat.ID,
			Name:             threat.Name,
			Description:      threat.Description,
			StateID:          state.ID,
			StateLabel:       state.Label,
			Severity:         state.Severity,
			StateDescription: state.Description,
		})
	}
	return out, nil
}

func matchingThreatState(threat Threat, view stateView) (ThreatState, error) {
	var matched ThreatState
	for _, state := range threat.States {
		if conditionIsZero(state.When) {
			matched = state
			continue
		}
		ok, err := evalCondition(state.When, view)
		if err != nil {
			return ThreatState{}, fmt.Errorf("state %s: %w", state.ID, err)
		}
		if ok {
			matched = state
		}
	}
	return matched, nil
}
