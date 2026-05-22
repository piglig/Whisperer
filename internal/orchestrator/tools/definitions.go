package tools

import "github.com/zhuzhenwu/whisperer/internal/agent"

// allToolDefs 返回完整 tool 定义。Schema 用 JSON Schema 子集。
//
// 顺序在 LLM 视图里固定，便于 prompt caching 命中。
func allToolDefs() []agent.ToolDefinition {
	return []agent.ToolDefinition{
		// ----- rules -----
		{
			Name:        "roll_skill",
			Description: "Resolve a CoC 7e skill check. The result is authoritative.",
			InputSchema: agent.ToolInputSchema{
				Properties: map[string]any{
					"skill_name":  obj("string", "Display name of the skill, e.g. 'Spot Hidden'."),
					"skill_value": obj("integer", "Investigator's current value for the skill, 0..99."),
					"difficulty": map[string]any{
						"type":        "string",
						"enum":        []string{"regular", "hard", "extreme"},
						"description": "Requested difficulty.",
					},
					"bonus_dice":   obj("integer", "0..2 bonus dice."),
					"penalty_dice": obj("integer", "0..2 penalty dice."),
				},
				Required: []string{"skill_name", "skill_value", "difficulty"},
			},
		},
		{
			Name:        "roll_damage",
			Description: "Roll a damage expression like '1d6+2'.",
			InputSchema: agent.ToolInputSchema{
				Properties: map[string]any{
					"expression": obj("string", "Dice expression, e.g. '1d6', '2d6+3', '1d10-1'."),
				},
				Required: []string{"expression"},
			},
		},
		{
			Name: "sanity_check",
			Description: "Resolve a SAN check for the active investigator. Reads current SAN from state, " +
				"applies the loss, persists the new SAN, and returns the trace including " +
				"whether indefinite insanity was triggered.",
			InputSchema: agent.ToolInputSchema{
				Properties: map[string]any{
					"loss_pass": obj("string", "Dice expression for SAN loss on success (e.g. '0', '1', '1d4')."),
					"loss_fail": obj("string", "Dice expression for SAN loss on failure (e.g. '1d4', '1d10')."),
				},
				Required: []string{"loss_pass", "loss_fail"},
			},
		},
		{
			Name:        "opposed_roll",
			Description: "Resolve an opposed check between two parties.",
			InputSchema: agent.ToolInputSchema{
				Properties: map[string]any{
					"actor_name":   obj("string", "Display name of the actor."),
					"actor_skill":  obj("integer", "Actor's skill value, 0..99."),
					"target_name":  obj("string", "Display name of the target."),
					"target_skill": obj("integer", "Target's skill value, 0..99."),
				},
				Required: []string{"actor_name", "actor_skill", "target_name", "target_skill"},
			},
		},

		// ----- read -----
		{
			Name:        "get_investigator",
			Description: "Return the current active investigator's full state.",
			InputSchema: agent.ToolInputSchema{Properties: map[string]any{}},
		},
		{
			Name:        "get_npc",
			Description: "Return an NPC's record by id.",
			InputSchema: agent.ToolInputSchema{
				Properties: map[string]any{
					"npc_id": obj("string", "NPC id."),
				},
				Required: []string{"npc_id"},
			},
		},
		{
			Name:        "get_location",
			Description: "Return a location record by id.",
			InputSchema: agent.ToolInputSchema{
				Properties: map[string]any{
					"location_id": obj("string", "Location id."),
				},
				Required: []string{"location_id"},
			},
		},
		{
			Name:        "list_npcs_at_location",
			Description: "List living NPCs at a given location.",
			InputSchema: agent.ToolInputSchema{
				Properties: map[string]any{
					"location_id": obj("string", "Location id."),
				},
				Required: []string{"location_id"},
			},
		},
		{
			Name:        "list_found_clues",
			Description: "List clues already discovered in this save.",
			InputSchema: agent.ToolInputSchema{Properties: map[string]any{}},
		},

		// ----- write -----
		{
			Name:        "update_investigator_vitals",
			Description: "Set HP / MP / SAN of the active investigator.",
			InputSchema: agent.ToolInputSchema{
				Properties: map[string]any{
					"investigator_id": obj("string", "Investigator id."),
					"hp":              obj("integer", "New HP (>= 0)."),
					"mp":              obj("integer", "New MP (>= 0)."),
					"san":             obj("integer", "New SAN (>= 0)."),
				},
				Required: []string{"investigator_id", "hp", "mp", "san"},
			},
		},
		{
			Name:        "update_npc_relation",
			Description: "Adjust an NPC's relation_to_player by delta.",
			InputSchema: agent.ToolInputSchema{
				Properties: map[string]any{
					"npc_id": obj("string", "NPC id."),
					"delta":  obj("integer", "Signed change to relation_to_player."),
				},
				Required: []string{"npc_id", "delta"},
			},
		},
		{
			Name:        "kill_npc",
			Description: "Mark an NPC as dead (alive=0). Does not delete the row.",
			InputSchema: agent.ToolInputSchema{
				Properties: map[string]any{
					"npc_id": obj("string", "NPC id."),
				},
				Required: []string{"npc_id"},
			},
		},
		{
			Name:        "mark_location_visited",
			Description: "Mark a location as visited.",
			InputSchema: agent.ToolInputSchema{
				Properties: map[string]any{
					"location_id": obj("string", "Location id."),
				},
				Required: []string{"location_id"},
			},
		},
		{
			Name:        "move_item",
			Description: "Change an item's owner.",
			InputSchema: agent.ToolInputSchema{
				Properties: map[string]any{
					"item_id":    obj("string", "Item id."),
					"owner_type": map[string]any{"type": "string", "enum": []string{"npc", "location", "investigator", "none"}, "description": "New owner kind."},
					"owner_id":   obj("string", "New owner id (empty when owner_type=none)."),
				},
				Required: []string{"item_id", "owner_type"},
			},
		},
		{
			Name:        "destroy_item",
			Description: "Mark an item as destroyed. Item conservation: do NOT revive it later.",
			InputSchema: agent.ToolInputSchema{
				Properties: map[string]any{
					"item_id": obj("string", "Item id."),
				},
				Required: []string{"item_id"},
			},
		},
		{
			Name:        "mark_clue_found",
			Description: "Record that a clue has been discovered.",
			InputSchema: agent.ToolInputSchema{
				Properties: map[string]any{
					"clue_id":     obj("string", "Clue id."),
					"location_id": obj("string", "Where it was found (optional)."),
				},
				Required: []string{"clue_id"},
			},
		},
		{
			Name:        "transition_location",
			Description: "Move the investigator's current location and advance the save's progress.",
			InputSchema: agent.ToolInputSchema{
				Properties: map[string]any{
					"location_id": obj("string", "Destination location id."),
				},
				Required: []string{"location_id"},
			},
		},
		{
			Name:        "get_time_of_day",
			Description: "Return the save's current time-of-day stage: morning|afternoon|night.",
			InputSchema: agent.ToolInputSchema{Properties: map[string]any{}},
		},
		{
			Name:        "advance_time",
			Description: "Advance the time-of-day by the given number of stages (1 = morning→afternoon).",
			InputSchema: agent.ToolInputSchema{
				Properties: map[string]any{
					"stages": obj("integer", "How many stages to advance (default 1)."),
				},
			},
		},
		{
			Name: "npc_speak",
			Description: "Voice an NPC in their own persona. Reads NPC profile + recent history from memory, " +
				"calls a sub-agent, returns one short utterance. The line is also appended to the event log. " +
				"Use this when the GM narrative needs the NPC to actually speak in-character.",
			InputSchema: agent.ToolInputSchema{
				Properties: map[string]any{
					"npc_id":      obj("string", "NPC id."),
					"intent":      obj("string", "Brief instruction to the NPC sub-agent (tone, topic, attitude)."),
					"player_line": obj("string", "Optional: the player line the NPC is responding to."),
				},
				Required: []string{"npc_id", "intent"},
			},
		},
		{
			Name:        "add_event",
			Description: "Append an event to the log. Use type='narrative' for in-fiction notes, 'state_change' for material changes already applied via other tools.",
			InputSchema: agent.ToolInputSchema{
				Properties: map[string]any{
					"type":             obj("string", "Event type, e.g. 'narrative', 'state_change'."),
					"description":      obj("string", "Short summary, ≤ 200 chars."),
					"related_entities": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional list of entity ids referenced."},
				},
				Required: []string{"type", "description"},
			},
		},
	}
}

// obj 是 schema 子结构的简短构造器。
func obj(typ, desc string) map[string]any {
	return map[string]any{"type": typ, "description": desc}
}
