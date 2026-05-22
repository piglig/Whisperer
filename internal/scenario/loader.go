package scenario

import (
	"embed"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

// sanLossPattern 校验形如 "0/1"、"1/1d4"、"1d3/1d10" 的克苏鲁 SAN 损失字符串。
var sanLossPattern = regexp.MustCompile(`^(\d+|\d*d\d+)/(\d+|\d*d\d+)$`)
var scenarioValidator = newScenarioValidator()

func newScenarioValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		name := strings.Split(field.Tag.Get("yaml"), ",")[0]
		if name == "-" {
			return ""
		}
		return name
	})
	_ = v.RegisterValidation("sanloss", func(fl validator.FieldLevel) bool {
		return sanLossPattern.MatchString(fl.Field().String())
	})
	return v
}

//go:embed data/*.yaml
var bundled embed.FS

type BundledInfo struct {
	ID        string
	Title     string
	Locations int
	NPCs      int
	Clues     int
	Variants  int
}

// ListBundled returns player-facing metadata for every embedded scenario.
func ListBundled() ([]BundledInfo, error) {
	entries, err := bundled.ReadDir("data")
	if err != nil {
		return nil, fmt.Errorf("scenario: list bundled: %w", err)
	}
	out := make([]BundledInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".yaml")
		scn, err := LoadBundled(id)
		if err != nil {
			return nil, err
		}
		out = append(out, BundledInfo{
			ID:        scn.ID,
			Title:     scn.Title,
			Locations: len(scn.Locations),
			NPCs:      len(scn.NPCs),
			Clues:     len(scn.Clues),
			Variants:  len(scn.Variants),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// LoadBundled 按 id 加载内置剧本，例如 "fog_harbor"。
func LoadBundled(id string) (*Scenario, error) {
	if strings.ContainsAny(id, "/\\.") {
		return nil, fmt.Errorf("scenario: invalid id %q", id)
	}
	data, err := bundled.ReadFile("data/" + id + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("scenario: read bundled %s: %w", id, err)
	}
	return Parse(data)
}

// Parse 把 YAML 字节流解析为 Scenario，并跑一遍 Validate。
func Parse(raw []byte) (*Scenario, error) {
	var s Scenario
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("scenario: parse: %w", err)
	}
	if err := Validate(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Validate 做静态检查：必填字段、ID 唯一性、引用完整性、union 正交性。
func Validate(s *Scenario) error {
	if err := validateFields(s); err != nil {
		return err
	}

	locs := uniqueIDs(idsOfLocations(s.Locations), "location")
	if err := locs; err != nil {
		return err
	}
	npcs := uniqueIDs(idsOfNPCs(s.NPCs), "npc")
	if err := npcs; err != nil {
		return err
	}
	clues := uniqueIDs(idsOfClues(s.Clues), "clue")
	if err := clues; err != nil {
		return err
	}
	items := uniqueIDs(idsOfItems(s.Items), "item")
	if err := items; err != nil {
		return err
	}
	triggers := uniqueIDs(idsOfTriggers(s.Triggers), "trigger")
	if err := triggers; err != nil {
		return err
	}
	endings := uniqueIDs(idsOfEndings(s.Endings), "ending")
	if err := endings; err != nil {
		return err
	}

	locSet := setOf(idsOfLocations(s.Locations))
	npcSet := setOf(idsOfNPCs(s.NPCs))
	clueSet := setOf(idsOfClues(s.Clues))

	if !locSet[s.Start.Location] {
		return fmt.Errorf("scenario: start.location %q not declared", s.Start.Location)
	}

	for _, l := range s.Locations {
		for _, c := range l.Connections {
			if !locSet[c] {
				return fmt.Errorf("scenario: location %s connects to unknown %q", l.ID, c)
			}
		}
	}
	seenObjectiveStages := map[string]bool{}
	for _, obj := range s.Objectives {
		if seenObjectiveStages[obj.Stage] {
			return fmt.Errorf("scenario: duplicate objective stage %q", obj.Stage)
		}
		seenObjectiveStages[obj.Stage] = true
	}
	if len(seenObjectiveStages) > 0 && !seenObjectiveStages[DefaultStage] {
		return fmt.Errorf("scenario: objectives must include %q stage", DefaultStage)
	}
	for _, n := range s.NPCs {
		if n.Location != "" && !locSet[n.Location] {
			return fmt.Errorf("scenario: npc %s references unknown location %q", n.ID, n.Location)
		}
		seenDialogueOptions := map[string]bool{}
		for _, opt := range n.DialogueOptions {
			if opt.ID == "" || opt.Label == "" || opt.Prompt == "" {
				return fmt.Errorf("scenario: npc %s dialogue option requires id, label and prompt", n.ID)
			}
			if seenDialogueOptions[opt.ID] {
				return fmt.Errorf("scenario: npc %s duplicate dialogue option %q", n.ID, opt.ID)
			}
			seenDialogueOptions[opt.ID] = true
			for _, stage := range opt.Stages {
				if len(seenObjectiveStages) > 0 && !seenObjectiveStages[stage] {
					return fmt.Errorf("scenario: npc %s dialogue option %s references unknown stage %q", n.ID, opt.ID, stage)
				}
			}
			for _, clue := range opt.RequiresClues {
				if !clueSet[clue] {
					return fmt.Errorf("scenario: npc %s dialogue option %s requires unknown clue %q", n.ID, opt.ID, clue)
				}
			}
			for _, clue := range opt.SuppressIfClues {
				if !clueSet[clue] {
					return fmt.Errorf("scenario: npc %s dialogue option %s suppresses unknown clue %q", n.ID, opt.ID, clue)
				}
			}
		}
	}
	for _, c := range s.Clues {
		if c.Location != "" && !locSet[c.Location] {
			return fmt.Errorf("scenario: clue %s references unknown location %q", c.ID, c.Location)
		}
		if c.Source != "" && !npcSet[c.Source] {
			return fmt.Errorf("scenario: clue %s references unknown source npc %q", c.ID, c.Source)
		}
	}
	for _, it := range s.Items {
		switch it.OwnerType {
		case "npc":
			if !npcSet[it.OwnerID] {
				return fmt.Errorf("scenario: item %s owner npc %q not declared", it.ID, it.OwnerID)
			}
		case "location":
			if !locSet[it.OwnerID] {
				return fmt.Errorf("scenario: item %s owner location %q not declared", it.ID, it.OwnerID)
			}
		case "investigator", "none":
			// no FK
		}
		seenItemActions := map[string]bool{}
		for _, action := range it.Actions {
			if action.ID == "" || action.Label == "" || action.Prompt == "" {
				return fmt.Errorf("scenario: item %s action requires id, label and prompt", it.ID)
			}
			if seenItemActions[action.ID] {
				return fmt.Errorf("scenario: item %s duplicate action %q", it.ID, action.ID)
			}
			seenItemActions[action.ID] = true
			for _, stage := range action.Stages {
				if len(seenObjectiveStages) > 0 && !seenObjectiveStages[stage] {
					return fmt.Errorf("scenario: item %s action %s references unknown stage %q", it.ID, action.ID, stage)
				}
			}
			for _, loc := range action.Locations {
				if !locSet[loc] {
					return fmt.Errorf("scenario: item %s action %s references unknown location %q", it.ID, action.ID, loc)
				}
			}
			for _, ownerType := range action.OwnerTypes {
				switch ownerType {
				case "npc", "location", "investigator", "none":
				default:
					return fmt.Errorf("scenario: item %s action %s has invalid owner_type %q", it.ID, action.ID, ownerType)
				}
			}
			for _, clue := range action.RequiresClues {
				if !clueSet[clue] {
					return fmt.Errorf("scenario: item %s action %s requires unknown clue %q", it.ID, action.ID, clue)
				}
			}
			for _, clue := range action.SuppressIfClues {
				if !clueSet[clue] {
					return fmt.Errorf("scenario: item %s action %s suppresses unknown clue %q", it.ID, action.ID, clue)
				}
			}
		}
	}
	for _, kc := range s.KeyClues {
		if !clueSet[kc] {
			return fmt.Errorf("scenario: key_clue %q not declared", kc)
		}
	}
	for _, t := range s.Triggers {
		if err := validateCondition(&t.When, locSet, npcSet, clueSet, idsOfTriggersSet(s.Triggers)); err != nil {
			return fmt.Errorf("trigger %s: %w", t.ID, err)
		}
		if len(t.Then) == 0 {
			return fmt.Errorf("trigger %s: empty actions", t.ID)
		}
		for i, a := range t.Then {
			if err := validateAction(a, npcSet, clueSet, seenObjectiveStages); err != nil {
				return fmt.Errorf("trigger %s action[%d]: %w", t.ID, i, err)
			}
		}
	}
	for _, e := range s.Endings {
		switch e.Kind {
		case "success", "failure":
		default:
			return fmt.Errorf("ending %s: kind must be success|failure", e.ID)
		}
		if err := validateCondition(&e.When, locSet, npcSet, clueSet, idsOfTriggersSet(s.Triggers)); err != nil {
			return fmt.Errorf("ending %s: %w", e.ID, err)
		}
	}
	if err := validateVariants(s, locSet, npcSet, clueSet, idsOfTriggersSet(s.Triggers), idsOfEndingsSet(s.Endings)); err != nil {
		return err
	}
	return nil
}

func validateVariants(s *Scenario, locs, npcs, clues, triggers, endings map[string]bool) error {
	if len(s.Variants) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, v := range s.Variants {
		if v.ID == "" {
			return fmt.Errorf("variant: empty id")
		}
		if seen[v.ID] {
			return fmt.Errorf("variant: duplicate id %q", v.ID)
		}
		seen[v.ID] = true
		if v.Culprit != "" && !npcs[v.Culprit] {
			return fmt.Errorf("variant %s: culprit references unknown npc %q", v.ID, v.Culprit)
		}
		for npcID := range v.NPCSecrets {
			if !npcs[npcID] {
				return fmt.Errorf("variant %s: npc_secrets references unknown npc %q", v.ID, npcID)
			}
		}
		for npcID := range v.NPCKnowledgeOverrides {
			if !npcs[npcID] {
				return fmt.Errorf("variant %s: npc_knowledge_overrides references unknown npc %q", v.ID, npcID)
			}
		}
		for clueID, patch := range v.ClueOverrides {
			if !clues[clueID] {
				return fmt.Errorf("variant %s: clue_overrides references unknown clue %q", v.ID, clueID)
			}
			if patch.Location != "" && !locs[patch.Location] {
				return fmt.Errorf("variant %s: clue %s patch references unknown location %q", v.ID, clueID, patch.Location)
			}
			if patch.Source != "" && !npcs[patch.Source] {
				return fmt.Errorf("variant %s: clue %s patch references unknown source npc %q", v.ID, clueID, patch.Source)
			}
		}
		for triggerID, patch := range v.TriggerOverrides {
			if !triggers[triggerID] {
				return fmt.Errorf("variant %s: trigger_overrides references unknown trigger %q", v.ID, triggerID)
			}
			cond := patch.When
			if err := validateCondition(&cond, locs, npcs, clues, triggers); err != nil {
				return fmt.Errorf("variant %s: trigger %s: %w", v.ID, triggerID, err)
			}
		}
		for endingID := range v.EndingDescOverrides {
			if !endings[endingID] {
				return fmt.Errorf("variant %s: ending_desc_overrides references unknown ending %q", v.ID, endingID)
			}
		}
	}
	return nil
}

func idsOfEndingsSet(xs []Ending) map[string]bool { return setOf(idsOfEndings(xs)) }

func validateFields(s *Scenario) error {
	if err := scenarioValidator.Struct(s); err != nil {
		return fmt.Errorf("scenario: field validation: %w", err)
	}
	return nil
}

func validateCondition(c *Condition, locs, npcs, clues, triggers map[string]bool) error {
	if c == nil {
		return fmt.Errorf("condition is nil")
	}
	count := 0
	if len(c.All) > 0 {
		count++
		for i := range c.All {
			if err := validateCondition(&c.All[i], locs, npcs, clues, triggers); err != nil {
				return fmt.Errorf("all[%d]: %w", i, err)
			}
		}
	}
	if len(c.Any) > 0 {
		count++
		for i := range c.Any {
			if err := validateCondition(&c.Any[i], locs, npcs, clues, triggers); err != nil {
				return fmt.Errorf("any[%d]: %w", i, err)
			}
		}
	}
	if c.Not != nil {
		count++
		if err := validateCondition(c.Not, locs, npcs, clues, triggers); err != nil {
			return fmt.Errorf("not: %w", err)
		}
	}
	if c.LocationVisited != "" {
		count++
		if !locs[c.LocationVisited] {
			return fmt.Errorf("location_visited references unknown %q", c.LocationVisited)
		}
	}
	if c.ClueFound != "" {
		count++
		if !clues[c.ClueFound] {
			return fmt.Errorf("clue_found references unknown %q", c.ClueFound)
		}
	}
	if c.NPCDead != "" {
		count++
		if !npcs[c.NPCDead] {
			return fmt.Errorf("npc_dead references unknown %q", c.NPCDead)
		}
	}
	if c.NPCRelationLT != nil {
		count++
		if !npcs[c.NPCRelationLT.NPC] {
			return fmt.Errorf("npc_relation_lt references unknown npc %q", c.NPCRelationLT.NPC)
		}
	}
	if c.NPCRelationGT != nil {
		count++
		if !npcs[c.NPCRelationGT.NPC] {
			return fmt.Errorf("npc_relation_gt references unknown npc %q", c.NPCRelationGT.NPC)
		}
	}
	if c.TimeOfDay != "" {
		count++
		switch c.TimeOfDay {
		case "morning", "afternoon", "night":
		default:
			return fmt.Errorf("invalid time_of_day %q", c.TimeOfDay)
		}
	}
	if c.TurnGE > 0 {
		count++
	}
	if c.TriggerFired != "" {
		count++
		if !triggers[c.TriggerFired] {
			return fmt.Errorf("trigger_fired references unknown %q", c.TriggerFired)
		}
	}
	if count == 0 {
		return fmt.Errorf("empty condition")
	}
	if count > 1 {
		return fmt.Errorf("condition is union: exactly one field must be set, got %d", count)
	}
	return nil
}

func validateAction(a Action, npcs, clues, stages map[string]bool) error {
	count := 0
	if a.AddEvent != nil {
		count++
		if a.AddEvent.Type == "" || a.AddEvent.Description == "" {
			return fmt.Errorf("add_event requires type and description")
		}
	}
	if a.MarkClueFound != nil {
		count++
		if !clues[a.MarkClueFound.ClueID] {
			return fmt.Errorf("mark_clue_found references unknown %q", a.MarkClueFound.ClueID)
		}
	}
	if a.UpdateNPCRelation != nil {
		count++
		if !npcs[a.UpdateNPCRelation.NPCID] {
			return fmt.Errorf("update_npc_relation references unknown npc %q", a.UpdateNPCRelation.NPCID)
		}
	}
	if a.KillNPC != "" {
		count++
		if !npcs[a.KillNPC] {
			return fmt.Errorf("kill_npc references unknown %q", a.KillNPC)
		}
	}
	if a.AdvanceTime > 0 {
		count++
	}
	if a.SetStage != "" {
		count++
		if len(stages) > 0 && !stages[a.SetStage] {
			return fmt.Errorf("set_stage references unknown stage %q", a.SetStage)
		}
	}
	if count == 0 {
		return fmt.Errorf("empty action")
	}
	if count > 1 {
		return fmt.Errorf("action is union: exactly one field must be set, got %d", count)
	}
	return nil
}

// helpers ---------------------------------------------------------------

func idsOfLocations(xs []SLocation) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = x.ID
	}
	return out
}
func idsOfNPCs(xs []SNPC) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = x.ID
	}
	return out
}
func idsOfClues(xs []SClue) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = x.ID
	}
	return out
}
func idsOfItems(xs []SItem) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = x.ID
	}
	return out
}
func idsOfTriggers(xs []Trigger) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = x.ID
	}
	return out
}
func idsOfTriggersSet(xs []Trigger) map[string]bool { return setOf(idsOfTriggers(xs)) }
func idsOfEndings(xs []Ending) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = x.ID
	}
	return out
}

func setOf(ids []string) map[string]bool {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func uniqueIDs(ids []string, kind string) error {
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" {
			return fmt.Errorf("scenario: empty %s id", kind)
		}
		if seen[id] {
			return fmt.Errorf("scenario: duplicate %s id %q", kind, id)
		}
		seen[id] = true
	}
	return nil
}
