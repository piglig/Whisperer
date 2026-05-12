// Package scenario loads CoC 7e scenarios from YAML and runs the trigger
// engine + drift detector against the live save state.
//
// 设计原则：YAML 是声明式数据；触发器条件是有限并集，加载时严格校验，运行时不允许
// 越界。"剧本逻辑" 不写在代码里，由数据驱动 + GM agent 负责叙事填充。
package scenario

// Scenario 是一份完整剧本。
type Scenario struct {
	ID        string      `yaml:"id" json:"id"`
	Title     string      `yaml:"title" json:"title"`
	Version   string      `yaml:"version" json:"version"`
	Locations []SLocation `yaml:"locations" json:"locations"`
	NPCs      []SNPC      `yaml:"npcs" json:"npcs"`
	Clues     []SClue     `yaml:"clues" json:"clues"`
	Items     []SItem     `yaml:"items,omitempty" json:"items,omitempty"`
	Start     Start       `yaml:"start" json:"start"`
	KeyClues  []string    `yaml:"key_clues" json:"key_clues"`
	Triggers  []Trigger   `yaml:"triggers,omitempty" json:"triggers,omitempty"`
	Endings   []Ending    `yaml:"endings,omitempty" json:"endings,omitempty"`
}

type SLocation struct {
	ID          string   `yaml:"id" json:"id"`
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	ParentID    string   `yaml:"parent,omitempty" json:"parent,omitempty"`
	Connections []string `yaml:"connections,omitempty" json:"connections,omitempty"`
}

type SNPC struct {
	ID               string            `yaml:"id" json:"id"`
	Name             string            `yaml:"name" json:"name"`
	Personality      string            `yaml:"personality" json:"personality"`
	Knowledge        map[string]string `yaml:"knowledge,omitempty" json:"knowledge,omitempty"`
	RelationToPlayer int               `yaml:"relation_to_player,omitempty" json:"relation_to_player,omitempty"`
	Location         string            `yaml:"location,omitempty" json:"location,omitempty"`
}

type SClue struct {
	ID          string `yaml:"id" json:"id"`
	Description string `yaml:"description" json:"description"`
}

type SItem struct {
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	OwnerType   string `yaml:"owner_type" json:"owner_type"` // npc|location|investigator|none
	OwnerID     string `yaml:"owner_id,omitempty" json:"owner_id,omitempty"`
}

// Start 描述剧本的初始状态。
type Start struct {
	Location  string `yaml:"location" json:"location"`
	TimeOfDay string `yaml:"time_of_day,omitempty" json:"time_of_day,omitempty"`
}

// Trigger 是一条 "条件 → 动作" 规则。MVP 全部 once（fired 后不再触发）。
type Trigger struct {
	ID    string    `yaml:"id" json:"id"`
	When  Condition `yaml:"when" json:"when"`
	Then  []Action  `yaml:"then" json:"then"`
}

// Ending 在 Engine.CheckEndings 被命中时进入失败/成功收束页。
type Ending struct {
	ID          string    `yaml:"id" json:"id"`
	Kind        string    `yaml:"kind" json:"kind"` // success|failure
	When        Condition `yaml:"when" json:"when"`
	Description string    `yaml:"description" json:"description"`
}

// Condition 是条件 union；恰好一个字段非零，加载时由 Validate 检查。
type Condition struct {
	All []Condition `yaml:"all,omitempty" json:"all,omitempty"`
	Any []Condition `yaml:"any,omitempty" json:"any,omitempty"`
	Not *Condition  `yaml:"not,omitempty" json:"not,omitempty"`

	LocationVisited string  `yaml:"location_visited,omitempty" json:"location_visited,omitempty"`
	ClueFound       string  `yaml:"clue_found,omitempty" json:"clue_found,omitempty"`
	NPCDead         string  `yaml:"npc_dead,omitempty" json:"npc_dead,omitempty"`
	NPCRelationLT   *RelChk `yaml:"npc_relation_lt,omitempty" json:"npc_relation_lt,omitempty"`
	NPCRelationGT   *RelChk `yaml:"npc_relation_gt,omitempty" json:"npc_relation_gt,omitempty"`
	TimeOfDay       string  `yaml:"time_of_day,omitempty" json:"time_of_day,omitempty"`
	TurnGE          int     `yaml:"turn_ge,omitempty" json:"turn_ge,omitempty"`

	// TriggerFired 在 Endings 中常用：检查某 trigger 是否已经发生过。
	TriggerFired string `yaml:"trigger_fired,omitempty" json:"trigger_fired,omitempty"`
}

// RelChk 是关系阈值检查 payload。
type RelChk struct {
	NPC   string `yaml:"npc" json:"npc"`
	Value int    `yaml:"value" json:"value"`
}

// Action 是动作 union；同样恰好一个非零。
type Action struct {
	AddEvent          *ActionAddEvent          `yaml:"add_event,omitempty" json:"add_event,omitempty"`
	MarkClueFound     *ActionMarkClueFound     `yaml:"mark_clue_found,omitempty" json:"mark_clue_found,omitempty"`
	UpdateNPCRelation *ActionUpdateNPCRelation `yaml:"update_npc_relation,omitempty" json:"update_npc_relation,omitempty"`
	KillNPC           string                   `yaml:"kill_npc,omitempty" json:"kill_npc,omitempty"`
	AdvanceTime       int                      `yaml:"advance_time,omitempty" json:"advance_time,omitempty"`
}

type ActionAddEvent struct {
	Type        string `yaml:"type" json:"type"`
	Description string `yaml:"description" json:"description"`
}

type ActionMarkClueFound struct {
	ClueID     string `yaml:"clue_id" json:"clue_id"`
	LocationID string `yaml:"location_id,omitempty" json:"location_id,omitempty"`
}

type ActionUpdateNPCRelation struct {
	NPCID string `yaml:"npc_id" json:"npc_id"`
	Delta int    `yaml:"delta" json:"delta"`
}
