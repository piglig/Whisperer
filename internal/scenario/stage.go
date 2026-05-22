package scenario

import (
	"strings"
)

const DefaultStage = "opening"

// NormalizeStage returns a declared objective stage, preserving scenario spelling.
func NormalizeStage(s *Scenario, stage string) string {
	stage = strings.TrimSpace(stage)
	if stage == "" {
		return ""
	}
	if s == nil || len(s.Objectives) == 0 {
		return stage
	}
	for _, obj := range s.Objectives {
		if strings.EqualFold(obj.Stage, stage) {
			return obj.Stage
		}
	}
	return ""
}

// ObjectiveForStage selects the objective matching stage, falling back to opening.
func ObjectiveForStage(s *Scenario, stage string) Objective {
	if s == nil || len(s.Objectives) == 0 {
		return Objective{}
	}
	stage = NormalizeStage(s, stage)
	if stage != "" {
		for _, obj := range s.Objectives {
			if strings.EqualFold(obj.Stage, stage) {
				return obj
			}
		}
	}
	for _, obj := range s.Objectives {
		if strings.EqualFold(obj.Stage, DefaultStage) || strings.EqualFold(obj.Stage, "act1") {
			return obj
		}
	}
	return s.Objectives[0]
}
