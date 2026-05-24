# Rules Engine

## Purpose

`internal/rules` implements deterministic d100 mechanics. It is pure Go logic:
no I/O, no store access, no LLM access, and no scenario imports.

## Public Surface

| File | Responsibility |
|---|---|
| `dice.go` | parse and roll `NdM+K` expressions |
| `skillcheck.go` | Call of Cthulhu-style d100 skill checks |
| `sanity.go` | SAN checks and loss rolls |
| `opposed.go` | opposed rolls |
| `combat.go` | initiative and simplified attacks |
| `growth.go` | post-case skill growth checks |
| `types.go` | result structs and shared enums |

## Design Rules

- All exported functions return structured results suitable for trace output.
- Randomness is injected through `rand.Rand`; tests must be deterministic.
- Rules functions do not mutate store state.
- LLM narrative must never be trusted as a rules result.
- Tool handlers are responsible for persisting rules results.

## Dice Expressions

Supported grammar:

```text
expr = term { ("+" | "-") term }
term = integer | integer "d" integer
```

Examples:

- `1d6`
- `1d10+2`
- `2d6-1`

Unsupported by design:

- exploding dice
- keep-highest / keep-lowest
- nested expressions
- Fudge dice

## Skill Checks

Skill checks use a d100 roll against a skill value.

| Difficulty | Threshold |
|---|---|
| regular | value |
| hard | value / 2 |
| extreme | value / 5 |

Degrees:

- `critical_success`: roll is 1
- `extreme_success`: roll <= value / 5
- `hard_success`: roll <= value / 2
- `regular_success`: roll <= value
- `failure`: roll above value
- `fumble`: 96+ when value < 50, or 100 when value >= 50

Bonus and penalty dice cancel before rolling. Remaining bonus dice choose the
lowest tens digit; remaining penalty dice choose the highest tens digit.

## Sanity

SAN checks roll d100 against current SAN, then roll either the pass loss or fail
loss expression. Applying the loss to investigator state is handled by the
orchestrator tool layer.

## Combat

Combat is intentionally narrow:

- initiative ordering
- attack skill check
- damage roll
- impale approximation for impaling weapons on extreme or critical success

Armor, dodge, block, and full tactical combat are outside the current scope.

## Growth

Growth is settled after a case. A skill that was successfully used during play
gets a growth check. If `1d100 > current skill`, the skill increases by `1d10`.

## Test Expectations

Rules changes should include table-driven tests for:

- boundary values
- deterministic RNG behavior
- malformed dice expressions
- fumbles and criticals
- bonus and penalty dice
- SAN loss parsing
- combat damage behavior
