package scenario

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/zhuzhenwu/whisperer/internal/store"
)

type CaseReport struct {
	Ending          Ending       `json:"ending"`
	VariantID       string       `json:"variant_id,omitempty"`
	Stage           string       `json:"stage"`
	CulpritID       string       `json:"culprit_id,omitempty"`
	CulpritName     string       `json:"culprit_name,omitempty"`
	FoundKeyClues   []ReportClue `json:"found_key_clues,omitempty"`
	MissingKeyClues []ReportClue `json:"missing_key_clues,omitempty"`
	FoundClues      []ReportClue `json:"found_clues,omitempty"`
	NPCOutcomes     []NPCOutcome `json:"npc_outcomes,omitempty"`
	TruthSummary    string       `json:"truth_summary,omitempty"`
	EvidenceStatus  string       `json:"evidence_status"`
}

type ReportClue struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Tier        int    `json:"tier,omitempty"`
	FoundAtTurn int    `json:"found_at_turn,omitempty"`
}

type NPCOutcome struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Alive    bool   `json:"alive"`
	Relation int    `json:"relation"`
}

type CaseReportInput struct {
	Save       store.Save
	Ending     *Ending
	VariantID  string
	FoundClues []store.Clue
	NPCs       []store.NPC
	TruthLimit int
}

func BuildCaseReport(s *Scenario, in CaseReportInput) *CaseReport {
	if s == nil || in.Ending == nil {
		return nil
	}
	found := make(map[string]store.Clue, len(in.FoundClues))
	for _, clue := range in.FoundClues {
		found[clue.ID] = clue
	}
	report := &CaseReport{
		Ending:       *in.Ending,
		VariantID:    in.VariantID,
		Stage:        NormalizeStage(s, in.Save.Stage),
		CulpritID:    s.Culprit,
		CulpritName:  npcName(s, s.Culprit),
		TruthSummary: truthSummary(s.Truth, in.TruthLimit),
	}
	if report.Stage == "" {
		report.Stage = DefaultStage
	}

	for _, id := range s.KeyClues {
		meta := reportClueMeta(s, id)
		if clue, ok := found[id]; ok {
			meta.FoundAtTurn = clue.FoundAtTurn
			report.FoundKeyClues = append(report.FoundKeyClues, meta)
		} else {
			report.MissingKeyClues = append(report.MissingKeyClues, meta)
		}
	}
	for _, clue := range in.FoundClues {
		meta := reportClueMeta(s, clue.ID)
		meta.FoundAtTurn = clue.FoundAtTurn
		report.FoundClues = append(report.FoundClues, meta)
	}
	sort.SliceStable(report.FoundClues, func(i, j int) bool {
		if report.FoundClues[i].Tier != report.FoundClues[j].Tier {
			return report.FoundClues[i].Tier > report.FoundClues[j].Tier
		}
		return report.FoundClues[i].FoundAtTurn < report.FoundClues[j].FoundAtTurn
	})

	npcs := make(map[string]store.NPC, len(in.NPCs))
	for _, npc := range in.NPCs {
		npcs[npc.ID] = npc
	}
	for _, snpc := range s.NPCs {
		npc, ok := npcs[snpc.ID]
		if !ok {
			continue
		}
		report.NPCOutcomes = append(report.NPCOutcomes, NPCOutcome{
			ID:       npc.ID,
			Name:     nonEmptyString(npc.Name, snpc.Name),
			Alive:    npc.Alive,
			Relation: npc.RelationToPlayer,
		})
	}
	report.EvidenceStatus = evidenceStatus(in.Ending, len(report.FoundKeyClues), len(s.KeyClues))
	return report
}

func reportClueMeta(s *Scenario, id string) ReportClue {
	for _, clue := range s.Clues {
		if clue.ID == id {
			return ReportClue{ID: id, Description: clue.Description, Tier: clue.Tier}
		}
	}
	return ReportClue{ID: id}
}

func npcName(s *Scenario, id string) string {
	if id == "" || s == nil {
		return ""
	}
	for _, npc := range s.NPCs {
		if npc.ID == id {
			return npc.Name
		}
	}
	return id
}

func truthSummary(truth string, limit int) string {
	truth = strings.TrimSpace(truth)
	if truth == "" {
		return ""
	}
	if limit <= 0 {
		limit = 360
	}
	truth = strings.Join(strings.Fields(truth), " ")
	if utf8.RuneCountInString(truth) <= limit {
		return truth
	}
	runes := []rune(truth)
	return string(runes[:limit]) + "..."
}

func evidenceStatus(ending *Ending, foundKey, totalKey int) string {
	if ending == nil {
		return ""
	}
	if ending.Kind == "failure" {
		return "调查失败：关键证据不足或局势失控。"
	}
	if totalKey > 0 && foundKey >= totalKey {
		return "关键证据完整：你已经拼齐主线证据链。"
	}
	if totalKey > 0 {
		return "主案成立，但证据链仍有缺口。"
	}
	return "已进入结局。"
}

func nonEmptyString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
