package scenario

import (
	"embed"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed data/*.yaml
var bundled embed.FS

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
	if s.ID == "" {
		return fmt.Errorf("scenario: missing id")
	}
	if s.Title == "" {
		return fmt.Errorf("scenario: missing title")
	}
	if len(s.Locations) == 0 {
		return fmt.Errorf("scenario: must have at least one location")
	}
	if s.Start.Location == "" {
		return fmt.Errorf("scenario: missing start.location")
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
	if s.Start.TimeOfDay != "" {
		switch s.Start.TimeOfDay {
		case "morning", "afternoon", "night":
		default:
			return fmt.Errorf("scenario: invalid start.time_of_day %q", s.Start.TimeOfDay)
		}
	}

	for _, l := range s.Locations {
		for _, c := range l.Connections {
			if !locSet[c] {
				return fmt.Errorf("scenario: location %s connects to unknown %q", l.ID, c)
			}
		}
	}
	for _, n := range s.NPCs {
		if n.Location != "" && !locSet[n.Location] {
			return fmt.Errorf("scenario: npc %s references unknown location %q", n.ID, n.Location)
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
		default:
			return fmt.Errorf("scenario: item %s has invalid owner_type %q", it.ID, it.OwnerType)
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
			if err := validateAction(a, npcSet, clueSet); err != nil {
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

func validateAction(a Action, npcs, clues map[string]bool) error {
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
