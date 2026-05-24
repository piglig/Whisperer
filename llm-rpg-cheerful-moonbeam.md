# Product Brief: LLM-Guided Investigative RPG

## Status

Historical product brief. The implementation has evolved beyond the first
implementation plan; use README and `specs/` as the current source of truth.

## Problem

Tabletop investigative RPGs require a GM, players, scheduling, rules knowledge,
and sustained attention. Most AI story tools can improvise prose, but they do
not reliably enforce rules, preserve state, or respect player failure.

Whisperer explores a stricter model:

- the LLM performs atmosphere, NPC voice, and scene narration
- deterministic code owns rules and state
- authoring tools verify scenario quality
- traces make failures inspectable

## Product Goals

- single-player local investigative RPG
- deterministic d100 rules
- persistent world state
- scenario-specific NPC secrets and knowledge boundaries
- clear blocked-action explanations
- replayable scenarios with variants and endings
- local trace and replay tooling for content iteration

## Non-Goals

- multiplayer
- mobile client
- generated images or voice
- full tactical combat simulator
- support for every tabletop rule system
- compatibility with early development saves or traces

## Current Product Shape

Whisperer now has three operating lanes:

| Lane | Purpose |
|---|---|
| Runtime | player-facing terminal game |
| Authoring | scenario lint, deterministic playtest, verify gates |
| Observability | e2e runs, trace replay, HTML viewer, reports |

The first bundled scenario is Fog Harbor.

## Core Experience

1. Player starts a save.
2. TUI shows the case board and actionable choices.
3. Player enters a natural-language action or selects an action.
4. Orchestrator validates whether the action is currently possible.
5. GM model narrates through tool calls.
6. Tools apply rules and state changes.
7. SLA and optional judge checks validate the result.
8. Turn recap explains what happened.
9. Trace is available for replay and debugging.

## Design Principles

- Failure must be playable and explained.
- Rules results must be inspectable.
- The model may narrate, but tools decide.
- Scenario authors need automated content gates.
- Debugging should happen through replay, not log archaeology.

## Current References

- [README](README.md)
- [Architecture overview](specs/00-architecture.md)
- [Scenario and triggers](specs/05-scenario-and-triggers.md)
- [Orchestrator and TUI](specs/06-orchestrator-and-tui.md)
- [Fog Harbor canon](specs/08-fog-harbor-canon.md)
