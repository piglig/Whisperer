# Changelog

All notable changes to Whisperer are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Apache-2.0 LICENSE.
- CONTRIBUTING / CODE_OF_CONDUCT / SECURITY documents.
- GitHub issue & pull request templates.
- README rewritten in product voice with Chaosium fan-material disclaimer.
- CHANGELOG (this file).

## [0.3.1] - 2026-05-13

### Changed
- **Variant system: role rotation instead of culprit swap.** All variants now
  share the same world (30-year sacrificial pact, seven historical victims,
  the same antagonist creature). Variants only swap *which current actor* is
  the active executor — fixing the world-consistency hole the previous design
  had (e.g. a "Marisa-revenge" variant that left seven historical bodies
  unexplained). New variants: `vance_executes` (default) / `calvin_directs` /
  `rourke_runs`.
- Renamed trigger `vance_confronted` → `culprit_confronted` (the confronted
  party varies by variant).
- `victim_dies` ending now also fires on `npc_dead: anna`.

### Added
- **Three Clue Rule redundancy.** Added `cloth_scrap` (harbor low-tide) /
  `empty_graves` (church graveyard) / `clinic_supplies` (pub) so each key
  conclusion has at least three independent clues.
- **Anna Rivers** — a 9th NPC representing the *next* sacrificial victim, with
  a `requires_phrases`-gated knowledge map and an `anna_taken` time-bomb
  trigger (turn ≥ 22 + culprit not yet confronted + relation < 26 → kills
  Anna). Gives the player a concrete face for the ticking clock instead of an
  abstract "another disappearance is coming".
- Auto-discovery triggers `cloth_at_low_tide`, `empty_graves_seen`, and
  `anna_warns`.

## [0.3.0] - 2026-05-13

### Added
- **Variant system** for replayability. Each playthrough randomly selects one
  variant; the variant patches truth / NPC secrets / clue overrides / trigger
  conditions / ending descriptions onto the base scenario, then merges into an
  effective scenario. `--variant <id>` and `--seed <n>` CLI flags allow
  forcing a deterministic selection.
- **Cross-run meta** in `runs/meta.json`: tracks completed variants, endings,
  and key truths discovered. Injected into the GM system prompt as "player
  prior" so NPCs can react with subtle déjà-vu without spoiling anything.
- **Keyword-gated NPC knowledge.** `SNPC.Knowledge` upgraded from `string` to a
  structured map with `requires_phrases` + `reveal` + optional `san_loss`.
  The NPC sub-agent decides whether the player's input triggers a reveal —
  the engine doesn't keyword-match.
- New scenario fields: `Scenario.Truth`, `SNPC.Secret`, `SClue.{Tier, Location,
  Source, SanLoss}`.
- `internal/scenario/{variant.go,meta.go,render.go}` plus tests.
- Store schema migration: `saves.variant_id` column. Saves are now
  variant-aware (re-merging on reload uses the persisted variant id).

### Changed
- `internal/scenario/data/fog_harbor.yaml` rewritten end-to-end. Six locations
  (added church + lucy_room + reef_cave) / eight NPCs (added father_calvin,
  deep_elder, lucy as plot anchor) / 12 clues across three tiers + one red
  herring / nine triggers (including two time-pressure triggers
  `helena_despairs` and `rourke_warns`) / five endings (`solved`,
  `pact_broken`, `flee_with_truth`, `victim_dies`, `dismissed`). Investigator
  death and indefinite insanity are handled by the orchestrator's existing
  death page, not as YAML endings.
- GM system prompt template gained sections for truth, NPC secrets, NPC
  hidden-knowledge keyword tables, the tiered clue atlas, and player prior.
- NPC system prompt template now receives `Self.Secret` and per-NPC
  `Knowledge` rendering.

## [0.1.0] - 2026-05-12

### Added
- Initial commit: full W1–W7 implementation in one push.
- `internal/rules` — pure dice / skill check / opposed / combat / sanity /
  growth functions.
- `internal/store` — SQLite persistence + single-file save format with
  embedded `schema.sql`.
- `internal/orchestrator/tools` — 20+ tools bridging rules / store / memory.
- `internal/agent` — Anthropic SDK wrapper, GM (Sonnet) tool-use loop, NPC
  (Haiku) sub-agent, prompt templates.
- `internal/memory` — chromem-go event / NPC / clue collections.
- `internal/scenario` — YAML loader, trigger engine, drift detector. Initial
  Fog Harbor skeleton (4 loc / 5 NPC / 3 clues / 2 triggers / 3 endings).
- `internal/orchestrator` — turn loop, four structural SLA validators,
  optional Haiku LLM-as-judge for SLA #3 (NPC consistency) and #7 (knowledge
  projection).
- `internal/tui` — bubbletea terminal UI with `/save`, `/load`, `/bind`,
  `/hint` slash commands.
- W7 polish: complete impale rules, multi-investigator binding, autosave
  checkpoint events.
- `cmd/whisperer` CLI + `cmd/e2esmoke` real-LLM end-to-end harness.

[Unreleased]: https://github.com/piglig/Whisperer/compare/v0.3.1...HEAD
[0.3.1]: https://github.com/piglig/Whisperer/releases/tag/v0.3.1
[0.3.0]: https://github.com/piglig/Whisperer/releases/tag/v0.3.0
[0.1.0]: https://github.com/piglig/Whisperer/releases/tag/v0.1.0
