# Orchestrator and TUI

## Purpose

`internal/orchestrator` coordinates one game turn. `internal/tui` presents that
loop to the player as a terminal game.

## Orchestrator Responsibilities

- parse player input into a `PlayerAction`
- validate action feasibility before contacting the LLM
- build the GM prompt
- run the GM tool-use loop
- execute tools inside a turn transaction
- validate SLA and judge checks
- evaluate scenario triggers and endings
- update memory
- append trace entries
- return a structured `TurnResult`

## TurnResult

`TurnResult` is the runtime contract between orchestrator, TUI, e2e validation,
and trace writer.

Important fields:

- narrative
- tool trace
- fired triggers
- drift status
- ending
- case report
- turn decision
- action summary
- SLA report
- save snapshot

## ActionDecision

The action guard produces a first-class `ActionDecision`.

```text
status: allowed | blocked | redirected
reason_code
player_facing_reason
debug_reason
required_clues
suggested_actions
```

Runtime shows the player-facing reason. Replay and authoring gates can inspect
the full structure.

Common blocked reasons:

- missing target
- disconnected location
- NPC not present
- item not accessible
- stale action-board option
- missing key clues for report/ending

## SLA and Retry

The orchestrator validates model output after tool execution. If an attempt
fails structural SLA validation, the transaction rolls back and the model can be
asked to regenerate with explicit feedback.

The final result records:

- structural violations
- judge checks
- forced ending status
- tool errors

## TUI Responsibilities

The TUI renders:

- case board
- actionable choices
- story timeline
- player input
- turn recap
- state diff
- rulings and blocked reasons
- endings and case report

The TUI must not call LLM providers directly. It talks to the orchestrator
through a runner interface.

## Slash Commands

| Command | Behavior |
|---|---|
| `/talk <NPC>` | target a specific NPC |
| `/all <text>` | speak to everyone present |
| `/hint` | ask for a non-spoiler nudge |
| `/bind <name> <occupation>` | bind a replacement investigator |
| `/help` | show help |

## Trace Writing

Trace writing is observability-only. Failure to write a trace should be logged
but must not fail the player turn.

Trace entries are consumed by `internal/replay`.

## Testing

Runtime changes should cover:

- action guard allowed and blocked paths
- rollback on SLA retry
- tool trace preservation
- ending and report generation
- TUI rendering of blocked decisions and recaps
