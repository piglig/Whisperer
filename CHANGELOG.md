# Changelog

All notable changes to Whisperer are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Whisperer is pre-1.0 and does not currently guarantee compatibility.

## [Unreleased]

### Added

- OpenAI-compatible semantic memory embedder with CLI/config controls and
  explicit fake/off modes.
- CLI/config switches for semantic SLA judge checks.
- TUI action board as the primary play surface.
- Per-turn recap entries for action, state changes, rulings, and blocked
  reasons.
- `whisperer e2e` for real-LLM validation without launching the TUI.
- `whisperer replay` for text trace replay, playtest script export, turn
  annotations, and single-file HTML replay viewer export.
- `internal/authoring` adapter registry, deterministic playtest environment,
  verify report assembly, and generic content gates.
- Structured `ActionDecision` with reason codes, player-facing explanations,
  debug reasons, required clues, and recovery suggestions.

### Changed

- CLI structure is organized into Runtime (`whisperer`), Authoring
  (`whisperer scenario ...`), and Observability (`whisperer e2e|replay`).
- Fog Harbor-specific command paths were folded into the generic scenario
  authoring CLI.
- Fog Harbor authoring code was narrowed to adapter, path scripts, and
  scenario-specific gates.
- Replay parsing and HTML rendering moved behind `internal/replay`.
- Director logic moved out of Fog Harbor-specific code.

### Removed

- Legacy runtime health flag.
- Legacy standalone e2e command.
- Scenario-specific top-level CLI command.
- General-purpose playtest and verify helpers from `internal/fogharbor`.

## [0.3.1] - 2026-05-13

### Added

- Additional Fog Harbor clue redundancy:
  - `cloth_scrap`
  - `empty_graves`
  - `clinic_supplies`
- Anna Rivers as the next potential victim.
- Auto-discovery triggers:
  - `cloth_at_low_tide`
  - `empty_graves_seen`
  - `anna_warns`

### Changed

- Fog Harbor variants now rotate the active executor instead of replacing the
  underlying culprit history.
- `vance_confronted` was renamed to `culprit_confronted`.
- `victim_dies` can fire when Anna dies.

## [0.3.0] - 2026-05-13

### Added

- Scenario variant system.
- Cross-run meta state in `runs/meta.json`.
- Keyword-gated NPC knowledge.
- Scenario truth, NPC secrets, clue tiers, clue sources, and SAN loss metadata.
- Store migration for `saves.variant_id`.

### Changed

- Fog Harbor was rewritten with expanded locations, NPCs, clues, triggers,
  endings, and variants.
- GM and NPC prompts now receive richer scenario truth and knowledge context.

## [0.1.0] - 2026-05-12

### Added

- Initial runtime implementation.
- Deterministic rules engine.
- SQLite store.
- LLM GM and NPC agent layer.
- Tool dispatcher.
- Semantic memory.
- Scenario loader and trigger engine.
- Orchestrator turn loop.
- Bubble Tea TUI.
- Initial Fog Harbor scenario.

[Unreleased]: https://github.com/piglig/Whisperer/compare/v0.3.1...HEAD
[0.3.1]: https://github.com/piglig/Whisperer/releases/tag/v0.3.1
[0.3.0]: https://github.com/piglig/Whisperer/releases/tag/v0.3.0
[0.1.0]: https://github.com/piglig/Whisperer/releases/tag/v0.1.0
