package scenario

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 进一步覆盖 validateCondition / validateAction 的引用错误分支。

func TestValidate_Condition_References(t *testing.T) {
	cases := map[string]string{
		"unknown location": `
id: x
title: t
locations: [{id: a, name: A, description: x}]
clues: []
npcs: []
start: {location: a}
key_clues: []
triggers:
  - id: t1
    when: {location_visited: missing}
    then: [{add_event: {type: x, description: y}}]`,
		"unknown clue": `
id: x
title: t
locations: [{id: a, name: A, description: x}]
clues: []
npcs: []
start: {location: a}
key_clues: []
triggers:
  - id: t1
    when: {clue_found: missing}
    then: [{add_event: {type: x, description: y}}]`,
		"unknown npc dead": `
id: x
title: t
locations: [{id: a, name: A, description: x}]
clues: []
npcs: []
start: {location: a}
key_clues: []
triggers:
  - id: t1
    when: {npc_dead: missing}
    then: [{add_event: {type: x, description: y}}]`,
		"unknown relation lt npc": `
id: x
title: t
locations: [{id: a, name: A, description: x}]
clues: []
npcs: []
start: {location: a}
key_clues: []
triggers:
  - id: t1
    when: {npc_relation_lt: {npc: missing, value: 0}}
    then: [{add_event: {type: x, description: y}}]`,
		"unknown relation gt npc": `
id: x
title: t
locations: [{id: a, name: A, description: x}]
clues: []
npcs: []
start: {location: a}
key_clues: []
triggers:
  - id: t1
    when: {npc_relation_gt: {npc: missing, value: 0}}
    then: [{add_event: {type: x, description: y}}]`,
		"invalid time_of_day": `
id: x
title: t
locations: [{id: a, name: A, description: x}]
clues: []
npcs: []
start: {location: a}
key_clues: []
triggers:
  - id: t1
    when: {time_of_day: never}
    then: [{add_event: {type: x, description: y}}]`,
		"unknown trigger fired": `
id: x
title: t
locations: [{id: a, name: A, description: x}]
clues: []
npcs: []
start: {location: a}
key_clues: []
triggers:
  - id: t1
    when: {trigger_fired: missing}
    then: [{add_event: {type: x, description: y}}]`,
		"unknown clue in mark_clue_found": `
id: x
title: t
locations: [{id: a, name: A, description: x}]
clues: []
npcs: []
start: {location: a}
key_clues: []
triggers:
  - id: t1
    when: {turn_ge: 1}
    then:
      - mark_clue_found: {clue_id: missing}`,
		"unknown npc in update_npc_relation": `
id: x
title: t
locations: [{id: a, name: A, description: x}]
clues: []
npcs: []
start: {location: a}
key_clues: []
triggers:
  - id: t1
    when: {turn_ge: 1}
    then:
      - update_npc_relation: {npc_id: missing, delta: 1}`,
		"unknown npc in kill_npc": `
id: x
title: t
locations: [{id: a, name: A, description: x}]
clues: []
npcs: []
start: {location: a}
key_clues: []
triggers:
  - id: t1
    when: {turn_ge: 1}
    then:
      - kill_npc: missing`,
		"add_event missing fields": `
id: x
title: t
locations: [{id: a, name: A, description: x}]
clues: []
npcs: []
start: {location: a}
key_clues: []
triggers:
  - id: t1
    when: {turn_ge: 1}
    then:
      - add_event: {type: ""}`,
		"empty action": `
id: x
title: t
locations: [{id: a, name: A, description: x}]
clues: []
npcs: []
start: {location: a}
key_clues: []
triggers:
  - id: t1
    when: {turn_ge: 1}
    then:
      - {}`,
		"trigger empty actions": `
id: x
title: t
locations: [{id: a, name: A, description: x}]
clues: []
npcs: []
start: {location: a}
key_clues: []
triggers:
  - id: t1
    when: {turn_ge: 1}
    then: []`,
		"item bad owner type": `
id: x
title: t
locations: [{id: a, name: A, description: x}]
clues: []
npcs: []
items: [{id: i1, name: I, description: x, owner_type: weird, owner_id: a}]
start: {location: a}
key_clues: []`,
		"item owner npc missing": `
id: x
title: t
locations: [{id: a, name: A, description: x}]
clues: []
npcs: []
items: [{id: i1, name: I, description: x, owner_type: npc, owner_id: missing}]
start: {location: a}
key_clues: []`,
		"item owner location missing": `
id: x
title: t
locations: [{id: a, name: A, description: x}]
clues: []
npcs: []
items: [{id: i1, name: I, description: x, owner_type: location, owner_id: missing}]
start: {location: a}
key_clues: []`,
		"key_clue missing": `
id: x
title: t
locations: [{id: a, name: A, description: x}]
clues: []
npcs: []
start: {location: a}
key_clues: [missing]`,
		"npc location missing": `
id: x
title: t
locations: [{id: a, name: A, description: x}]
npcs: [{id: n1, name: N, personality: p, location: nope}]
clues: []
start: {location: a}
key_clues: []`,
	}
	for name, y := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(y))
			assert.Error(t, err)
		})
	}
}

func TestValidate_NestedAllAny(t *testing.T) {
	y := `
id: x
title: t
locations: [{id: a, name: A, description: x}]
clues: [{id: c1, description: z}]
npcs: []
start: {location: a}
key_clues: []
triggers:
  - id: t1
    when:
      all:
        - location_visited: a
        - any:
            - clue_found: c1
            - not: {turn_ge: 99}
    then:
      - add_event: {type: scenario, description: ok}
`
	s, err := Parse([]byte(y))
	assert.NoError(t, err)
	assert.NotNil(t, s)
}
