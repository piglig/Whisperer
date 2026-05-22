package store

// 领域模型。JSON 字段以原始字符串持久化，由上层（agent / orchestrator）按需反序列化。
//
// 时间戳字段：unix milliseconds。

// TimeOfDay 是 saves.time_of_day 的合法取值。
type TimeOfDay string

const (
	TimeMorning   TimeOfDay = "morning"
	TimeAfternoon TimeOfDay = "afternoon"
	TimeNight     TimeOfDay = "night"
)

// IsValid 校验值是否为合法 TimeOfDay。
func (t TimeOfDay) IsValid() bool {
	switch t {
	case TimeMorning, TimeAfternoon, TimeNight:
		return true
	}
	return false
}

// Next 返回三段制下的下一段。night → morning。
func (t TimeOfDay) Next() TimeOfDay {
	switch t {
	case TimeMorning:
		return TimeAfternoon
	case TimeAfternoon:
		return TimeNight
	case TimeNight:
		return TimeMorning
	}
	return TimeMorning
}

type Save struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	ScenarioID        string    `json:"scenario_id"`
	VariantID         string    `json:"variant_id,omitempty"`
	Stage             string    `json:"stage"`
	CurrentLocationID string    `json:"current_location_id,omitempty"`
	TurnCount         int       `json:"turn_count"`
	TimeOfDay         TimeOfDay `json:"time_of_day"`
	CreatedAt         int64     `json:"created_at"`
	UpdatedAt         int64     `json:"updated_at"`
}

type Investigator struct {
	ID            string `json:"id"`
	SaveID        string `json:"save_id"`
	Name          string `json:"name"`
	Occupation    string `json:"occupation"`
	AttrsJSON     string `json:"attrs_json"`
	SkillsJSON    string `json:"skills_json"`
	HP            int    `json:"hp"`
	MP            int    `json:"mp"`
	SAN           int    `json:"san"`
	InventoryJSON string `json:"inventory_json"`
	Active        bool   `json:"active"`
}

type NPC struct {
	ID               string `json:"id"`
	SaveID           string `json:"save_id"`
	Name             string `json:"name"`
	Personality      string `json:"personality"`
	KnowledgeJSON    string `json:"knowledge_json"`
	RelationToPlayer int    `json:"relation_to_player"`
	LocationID       string `json:"location_id,omitempty"`
	Alive            bool   `json:"alive"`
}

type Location struct {
	ID              string `json:"id"`
	SaveID          string `json:"save_id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	ParentID        string `json:"parent_id,omitempty"`
	ConnectionsJSON string `json:"connections_json"`
	Visited         bool   `json:"visited"`
}

// OwnerType 枚举。store 层只校验合法值，语义由上层决定。
type OwnerType string

const (
	OwnerNPC          OwnerType = "npc"
	OwnerLocation     OwnerType = "location"
	OwnerInvestigator OwnerType = "investigator"
	OwnerNone         OwnerType = "none"
)

type Item struct {
	ID             string    `json:"id"`
	SaveID         string    `json:"save_id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	OwnerType      OwnerType `json:"owner_type"`
	OwnerID        string    `json:"owner_id,omitempty"`
	PropertiesJSON string    `json:"properties_json"`
	Destroyed      bool      `json:"destroyed"`
}

type Clue struct {
	ID                string `json:"id"`
	SaveID            string `json:"save_id"`
	ScenarioID        string `json:"scenario_id"`
	Description       string `json:"description"`
	Found             bool   `json:"found"`
	FoundInLocationID string `json:"found_in_location_id,omitempty"`
	FoundAtTurn       int    `json:"found_at_turn,omitempty"`
}

// EventType 是 events.type 列的常用取值；store 不强制约束，仅作 trace 文档。
type EventType string

const (
	EventNarrative          EventType = "narrative"
	EventToolCall           EventType = "tool_call"
	EventStateChange        EventType = "state_change"
	EventDiceRoll           EventType = "dice_roll"
	EventScenario           EventType = "scenario"
	EventAutosaveCheckpoint EventType = "autosave_checkpoint"
)

type Event struct {
	ID                  int64     `json:"id"`
	SaveID              string    `json:"save_id"`
	Turn                int       `json:"turn"`
	Type                EventType `json:"type"`
	Description         string    `json:"description"`
	RelatedEntitiesJSON string    `json:"related_entities_json"`
	CreatedAt           int64     `json:"created_at"`
}
