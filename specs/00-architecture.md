# Architecture Overview

## Purpose

This document defines the stable architecture vocabulary for Whisperer. It is
the entry point for maintainers who need to understand where code belongs and
how the main runtime, authoring, and observability paths interact.

## System Lanes

Whisperer is organized around three lanes.

| Lane | User-facing entry | Internal path | Ownership |
|---|---|---|---|
| Runtime | `whisperer` | TUI -> orchestrator -> tools -> store/rules/scenario/memory | player experience |
| Authoring | `whisperer scenario lint|playtest|verify` | authoring adapter -> scenario/director/playtest gates | scenario quality |
| Observability | `whisperer e2e`, `whisperer replay` | trace JSONL -> replay/report/viewer | debugging and review |

Code that does not support one of these lanes should be removed, merged into a
lane, or isolated as test tooling.

## Package Map

```text
cmd/whisperer              CLI and TUI entrypoint
internal/agent             LLM provider adapters, GM/NPC agents, cassettes
internal/authoring         scenario adapter registry, playtest env, verify gates
internal/config            config file, environment, and flag resolution
internal/director          scenario pacing advice
internal/fogharbor         Fog Harbor adapter and path scripts
internal/i18n              localized UI/CLI strings
internal/investigator      investigator templates
internal/log               structured logging and redaction
internal/memory            semantic memory and embedders
internal/orchestrator      turn loop, action guard, SLA, tools, reports
internal/replay            trace document model and HTML replay viewer
internal/rules             deterministic d100 mechanics
internal/scenario          YAML loader, variants, triggers, reports
internal/secrets           secret redaction
internal/store             SQLite persistence and repository API
internal/tui               Bubble Tea terminal interface
```

## Dependency Direction

Runtime dependency flow:

```text
tui -> orchestrator -> tools -> {agent, store, memory, scenario} -> rules
```

Important constraints:

- `internal/rules` must remain pure and must not import sibling internal
  packages.
- `internal/agent` must not read or write game state directly.
- state changes visible to gameplay must go through orchestrator tools or the
  authoring playtest environment.
- scenario-specific authoring code should live behind an
  `authoring.ScenarioAdapter`.
- observability should consume trace data rather than reaching back into runtime
  implementation details.

## Runtime Turn Pipeline

```text
player input
  -> parse PlayerAction
  -> action guard / ActionDecision
  -> render GM prompt with scenario, memory, director context
  -> GM tool-use loop
  -> transactional tool execution
  -> SLA validation and optional judge checks
  -> trigger and ending evaluation
  -> memory update
  -> trace append
  -> TurnResult for TUI
```

Blocked actions return before the LLM call. They include a structured
`ActionDecision`:

```text
status: allowed | blocked | redirected
reason_code
player_facing_reason
debug_reason
required_clues
suggested_actions
```

## Authoring Pipeline

```text
scenario YAML
  -> scenario lint
  -> authoring.ScenarioAdapter
  -> deterministic playtest paths
  -> generic gates
  -> scenario-specific gates
  -> verify report
```

Generic gates cover lint cleanliness, mainline variant coverage, key clue
coverage, stage coverage, trigger activity, and blocked-action clarity.
Scenario adapters add scenario-specific gates such as expected ending coverage.

## Observability Pipeline

```text
orchestrator TraceEntry JSONL
  -> internal/replay Document
  -> text replay
  -> HTML viewer
  -> exported playtest script
  -> annotations / marks
```

Trace entries are the observability contract. The replay package should remain
able to render useful output without opening the SQLite store.

## Persistence

Whisperer keeps local state in three places:

| Data | Location |
|---|---|
| game state | SQLite database |
| semantic memory | `memory_dir` |
| traces and meta state | `runs/` by default |

Secrets must not be written to traces, logs, cassettes, or config files.

## Related Documents

- [Rules engine](01-rules-engine.md)
- [State store](02-store.md)
- [Tools and agent](03-tools-and-agent.md)
- [Memory and RAG](04-memory-and-rag.md)
- [Scenario and triggers](05-scenario-and-triggers.md)
- [Orchestrator and TUI](06-orchestrator-and-tui.md)
- [Quality and release](07-polish.md)
