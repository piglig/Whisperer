// Package scenario loads CoC 7e scenarios from YAML and runs the trigger
// engine + drift detector against the live save state.
//
// 设计原则：YAML 是声明式数据；触发器条件是有限并集，加载时严格校验，运行时不允许
// 越界。"剧本逻辑" 不写在代码里，由数据驱动 + GM agent 负责叙事填充。
package scenario

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Scenario 是一份完整剧本。
type Scenario struct {
	ID         string      `yaml:"id" json:"id" validate:"required"`
	Title      string      `yaml:"title" json:"title" validate:"required"`
	Version    string      `yaml:"version" json:"version"`
	Intro      string      `yaml:"intro,omitempty" json:"intro,omitempty"`
	Objectives []Objective `yaml:"objectives,omitempty" json:"objectives,omitempty" validate:"dive"`
	Threats    []Threat    `yaml:"threats,omitempty" json:"threats,omitempty" validate:"dive"`
	Culprit    string      `yaml:"culprit,omitempty" json:"culprit,omitempty"`
	Truth      string      `yaml:"truth,omitempty" json:"truth,omitempty"`
	Locations  []SLocation `yaml:"locations" json:"locations" validate:"required,min=1,dive"`
	NPCs       []SNPC      `yaml:"npcs" json:"npcs" validate:"dive"`
	Clues      []SClue     `yaml:"clues" json:"clues" validate:"dive"`
	Items      []SItem     `yaml:"items,omitempty" json:"items,omitempty" validate:"dive"`
	Start      Start       `yaml:"start" json:"start" validate:"required"`
	KeyClues   []string    `yaml:"key_clues" json:"key_clues"`
	Triggers   []Trigger   `yaml:"triggers,omitempty" json:"triggers,omitempty" validate:"dive"`
	Endings    []Ending    `yaml:"endings,omitempty" json:"endings,omitempty" validate:"dive"`
	Variants   []Variant   `yaml:"variants,omitempty" json:"variants,omitempty" validate:"dive"`
}

type SLocation struct {
	ID          string   `yaml:"id" json:"id"`
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Leads       []string `yaml:"leads,omitempty" json:"leads,omitempty"`
	ParentID    string   `yaml:"parent,omitempty" json:"parent,omitempty"`
	Connections []string `yaml:"connections,omitempty" json:"connections,omitempty"`
}

type Objective struct {
	Stage string   `yaml:"stage" json:"stage" validate:"required"`
	Title string   `yaml:"title" json:"title" validate:"required"`
	Steps []string `yaml:"steps,omitempty" json:"steps,omitempty"`
}

type Threat struct {
	ID          string        `yaml:"id" json:"id" validate:"required"`
	Name        string        `yaml:"name" json:"name" validate:"required"`
	Description string        `yaml:"description,omitempty" json:"description,omitempty"`
	States      []ThreatState `yaml:"states" json:"states" validate:"required,min=1,dive"`
}

type ThreatState struct {
	ID          string    `yaml:"id" json:"id" validate:"required"`
	Label       string    `yaml:"label" json:"label" validate:"required"`
	Severity    int       `yaml:"severity,omitempty" json:"severity,omitempty" validate:"gte=0,lte=4"`
	Description string    `yaml:"description,omitempty" json:"description,omitempty"`
	When        Condition `yaml:"when,omitempty" json:"when,omitempty"`
}

type SNPC struct {
	ID               string                  `yaml:"id" json:"id"`
	Name             string                  `yaml:"name" json:"name"`
	Personality      string                  `yaml:"personality" json:"personality"`
	FirstImpression  string                  `yaml:"first_impression,omitempty" json:"first_impression,omitempty"`
	OpeningLine      string                  `yaml:"opening_line,omitempty" json:"opening_line,omitempty"`
	DialogueOptions  []DialogueOption        `yaml:"dialogue_options,omitempty" json:"dialogue_options,omitempty" validate:"dive"`
	Secret           string                  `yaml:"secret,omitempty" json:"secret,omitempty"`
	Knowledge        map[string]NPCKnowledge `yaml:"knowledge,omitempty" json:"knowledge,omitempty"`
	RelationToPlayer int                     `yaml:"relation_to_player,omitempty" json:"relation_to_player,omitempty"`
	Location         string                  `yaml:"location,omitempty" json:"location,omitempty"`
}

type DialogueOption struct {
	ID              string   `yaml:"id" json:"id" validate:"required"`
	Label           string   `yaml:"label" json:"label" validate:"required"`
	Prompt          string   `yaml:"prompt" json:"prompt" validate:"required"`
	Stages          []string `yaml:"stages,omitempty" json:"stages,omitempty"`
	RequiresClues   []string `yaml:"requires_clues,omitempty" json:"requires_clues,omitempty"`
	SuppressIfClues []string `yaml:"suppress_if_clues,omitempty" json:"suppress_if_clues,omitempty"`
}

// NPCKnowledge 是 NPC 持有的一条隐藏知识。requires_phrases 非空时表示玩家必须用其中
// 任一关键词触碰，NPC 才在叙事里"松口"。引擎不做关键词匹配——这是给 NPC 子代理（LLM）
// 的提示，由它自己判断玩家输入是否击中。
//
// YAML 支持两种写法：
//
//	# 短形式（仅 reveal，无关键词门槛）
//	knowledge:
//	  missing_person: 知道但不愿说
//
//	# 长形式（含关键词与 SAN 损失）
//	knowledge:
//	  sacrifice_history:
//	    requires_phrases: ["献祭", "古约"]
//	    reveal: 我父亲那一辈也提过……
//	    san_loss: "0/1d3"
type NPCKnowledge struct {
	RequiresPhrases []string `yaml:"requires_phrases,omitempty" json:"requires_phrases,omitempty"`
	Reveal          string   `yaml:"reveal" json:"reveal" validate:"required"`
	SanLoss         string   `yaml:"san_loss,omitempty" json:"san_loss,omitempty" validate:"omitempty,sanloss"`
}

// UnmarshalYAML 让 NPCKnowledge 既能从字符串解析（短形式），也能从 mapping 解析。
func (k *NPCKnowledge) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		k.Reveal = value.Value
		return nil
	}
	if value.Kind == yaml.MappingNode {
		// 用一个不同结构体名做 Decode，避免无限递归 UnmarshalYAML。
		type rawKnowledge struct {
			RequiresPhrases []string `yaml:"requires_phrases"`
			Reveal          string   `yaml:"reveal"`
			SanLoss         string   `yaml:"san_loss"`
		}
		var raw rawKnowledge
		if err := value.Decode(&raw); err != nil {
			return err
		}
		k.RequiresPhrases = raw.RequiresPhrases
		k.Reveal = raw.Reveal
		k.SanLoss = raw.SanLoss
		return nil
	}
	return fmt.Errorf("scenario: knowledge entry must be string or mapping, got kind %v", value.Kind)
}

type SClue struct {
	ID          string `yaml:"id" json:"id"`
	Description string `yaml:"description" json:"description"`
	Tier        int    `yaml:"tier,omitempty" json:"tier,omitempty"` // 1/2/3=主线层级；0 视为 red herring
	Location    string `yaml:"location,omitempty" json:"location,omitempty"`
	Source      string `yaml:"source,omitempty" json:"source,omitempty"`                                  // 哪个 NPC 持有；可空
	SanLoss     string `yaml:"san_loss,omitempty" json:"san_loss,omitempty" validate:"omitempty,sanloss"` // "0/1" 或 "1/1d4"
}

type SItem struct {
	ID          string       `yaml:"id" json:"id"`
	Name        string       `yaml:"name" json:"name"`
	Description string       `yaml:"description" json:"description"`
	OwnerType   string       `yaml:"owner_type" json:"owner_type" validate:"oneof=npc location investigator none"` // npc|location|investigator|none
	OwnerID     string       `yaml:"owner_id,omitempty" json:"owner_id,omitempty"`
	Actions     []ItemAction `yaml:"actions,omitempty" json:"actions,omitempty" validate:"dive"`
}

type ItemAction struct {
	ID              string   `yaml:"id" json:"id" validate:"required"`
	Label           string   `yaml:"label" json:"label" validate:"required"`
	Prompt          string   `yaml:"prompt" json:"prompt" validate:"required"`
	Stages          []string `yaml:"stages,omitempty" json:"stages,omitempty"`
	Locations       []string `yaml:"locations,omitempty" json:"locations,omitempty"`
	OwnerTypes      []string `yaml:"owner_types,omitempty" json:"owner_types,omitempty"`
	RequiresClues   []string `yaml:"requires_clues,omitempty" json:"requires_clues,omitempty"`
	SuppressIfClues []string `yaml:"suppress_if_clues,omitempty" json:"suppress_if_clues,omitempty"`
}

// Start 描述剧本的初始状态。
type Start struct {
	Location  string `yaml:"location" json:"location" validate:"required"`
	TimeOfDay string `yaml:"time_of_day,omitempty" json:"time_of_day,omitempty" validate:"omitempty,oneof=morning afternoon night"`
}

// Trigger 是一条 "条件 → 动作" 规则。MVP 全部 once（fired 后不再触发）。
type Trigger struct {
	ID   string    `yaml:"id" json:"id"`
	When Condition `yaml:"when" json:"when"`
	Then []Action  `yaml:"then" json:"then"`
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

	LocationVisited string   `yaml:"location_visited,omitempty" json:"location_visited,omitempty"`
	CurrentLocation string   `yaml:"current_location,omitempty" json:"current_location,omitempty"`
	LocationIn      []string `yaml:"location_in,omitempty" json:"location_in,omitempty"` // 玩家当前位置 ∈ 列表中任一
	ClueFound       string  `yaml:"clue_found,omitempty" json:"clue_found,omitempty"`
	NPCDead         string  `yaml:"npc_dead,omitempty" json:"npc_dead,omitempty"`
	NPCRelationLT   *RelChk `yaml:"npc_relation_lt,omitempty" json:"npc_relation_lt,omitempty"`
	NPCRelationGT   *RelChk `yaml:"npc_relation_gt,omitempty" json:"npc_relation_gt,omitempty"`
	TimeOfDay       string  `yaml:"time_of_day,omitempty" json:"time_of_day,omitempty" validate:"omitempty,oneof=morning afternoon night"`
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
	SetStage          string                   `yaml:"set_stage,omitempty" json:"set_stage,omitempty"`
}

type ActionAddEvent struct {
	Type        string `yaml:"type" json:"type" validate:"required"`
	Description string `yaml:"description" json:"description" validate:"required"`
}

type ActionMarkClueFound struct {
	ClueID     string `yaml:"clue_id" json:"clue_id"`
	LocationID string `yaml:"location_id,omitempty" json:"location_id,omitempty"`
}

type ActionUpdateNPCRelation struct {
	NPCID string `yaml:"npc_id" json:"npc_id"`
	Delta int    `yaml:"delta" json:"delta"`
}

// ============================================================================
// Variant —— 支持每局随机选一个真凶/共谋/线索分布的"剧本变体"
// ============================================================================

// Variant 描述一个剧本变体。每局开始随机选一个，把 patch merge 到 base 后得到
// effective scenario。base 不变；variant 只 patch：
//   - 每个 NPC 的 secret 与 knowledge 条目
//   - 关键线索的 location/source 与描述
//   - 触发器的条件
//   - 结局的描述（保留 ID 与 kind 不变）
//
// 玩家不可见 variant 选择——结局页才显示"本局真凶"。
type Variant struct {
	ID                    string                             `yaml:"id" json:"id"`
	Weight                int                                `yaml:"weight,omitempty" json:"weight,omitempty" validate:"gte=0"`
	Truth                 string                             `yaml:"truth,omitempty" json:"truth,omitempty"`
	Culprit               string                             `yaml:"culprit,omitempty" json:"culprit,omitempty"`
	NPCSecrets            map[string]string                  `yaml:"npc_secrets,omitempty" json:"npc_secrets,omitempty"`
	NPCKnowledgeOverrides map[string]map[string]NPCKnowledge `yaml:"npc_knowledge_overrides,omitempty" json:"npc_knowledge_overrides,omitempty"`
	ClueOverrides         map[string]CluePatch               `yaml:"clue_overrides,omitempty" json:"clue_overrides,omitempty"`
	TriggerOverrides      map[string]ConditionPatch          `yaml:"trigger_overrides,omitempty" json:"trigger_overrides,omitempty"`
	EndingDescOverrides   map[string]string                  `yaml:"ending_desc_overrides,omitempty" json:"ending_desc_overrides,omitempty"`
}

// CluePatch 仅 patch 已声明 clue 的描述/位置/来源/SAN 损失文本。
// 不允许新增 clue id（避免 variant 引入引擎未知的引用）。
type CluePatch struct {
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Location    string `yaml:"location,omitempty" json:"location,omitempty"`
	Source      string `yaml:"source,omitempty" json:"source,omitempty"`
	SanLoss     string `yaml:"san_loss,omitempty" json:"san_loss,omitempty" validate:"omitempty,sanloss"`
}

// ConditionPatch 替换某 trigger 的 When 条件。整体替换而非合并——条件树语义复杂，
// 局部 patch 容易出歧义。
type ConditionPatch struct {
	When Condition `yaml:"when" json:"when"`
}
