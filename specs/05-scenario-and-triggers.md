# Scenario and Triggers

## Purpose

`internal/scenario` loads bundled YAML scenarios, applies variants, evaluates
triggers and endings, renders GM context, and reports case outcomes.

## Scenario YAML

Scenario files live in `internal/scenario/data/`.

Top-level fields:

```yaml
id: fog_harbor
title: 雾港疑案
version: 0.3.1
truth: "GM-only scenario truth"
start:
  location: harbor
  time_of_day: morning
key_clues: [blood_letter, tide_chart]
locations: []
npcs: []
items: []
clues: []
triggers: []
endings: []
variants: []
```

Scenario IDs are stable identifiers used by saves, traces, configs, and
authoring adapters.

## Locations

Locations define the playable map:

```yaml
- id: harbor
  name: 雾港码头
  description: ...
  connections: [pub, lighthouse]
  leads: []
```

Connections control movement validation. Empty connections mean the location is
not movement-gated by adjacency.

## NPCs

NPCs include:

- ID and display name
- persona
- secret
- starting relation
- starting location
- knowledge entries gated by phrases or discovered clues
- dialogue options for the action board

NPC knowledge is GM-facing. Player-facing reveals must happen through dialogue,
tools, or discovered clues.

## Items

Items may belong to:

- investigator
- location
- NPC
- no owner

Item actions can be gated by stage, location, ownership, and discovered clues.
Blocked item actions must provide a clear reason and recovery path.

## Clues

Clues include:

- tier
- location/source metadata
- optional SAN loss
- description

Scenario authors should follow the Three Clue Rule: every key conclusion needs
at least three independent clue paths.

## Triggers

Triggers are declarative rules evaluated after turns and during deterministic
playtests.

Common condition categories:

- clue found
- location visited
- NPC relation threshold
- NPC alive/dead
- turn count
- time of day
- trigger already fired / not fired

Common actions:

- add event
- mark clue found
- update stage
- move NPC
- change relationship
- set threat state

Triggers should be deterministic and idempotent. A trigger should fire once
unless explicitly designed otherwise.

## Endings

Endings are scenario-defined terminal outcomes. Runtime may also force system
endings when investigator HP or SAN reaches a terminal condition.

Ending reports should include:

- ending ID and kind
- evidence status
- culprit status
- found and missing key clues
- trigger summary

## Variants

Variants patch the base scenario at load time. Fog Harbor uses variants to rotate
the active executor while preserving the same world history.

Variant rules:

- keep base truth coherent
- patch only the fields that differ
- keep deterministic authoring coverage for every variant that affects endings

## Authoring Adapter

Every scenario that supports playtest/verify should register an
`authoring.ScenarioAdapter`.

The adapter owns:

- path validation
- deterministic playtest path scripts
- scenario-specific gates

Generic authoring infrastructure owns:

- in-memory playtest environment
- variant selection
- report generation
- lint gate
- mainline variant gate
- key clue gate
- stage gate
- trigger activity gate
- blocked action clarity gate

## Fog Harbor

Fog Harbor lives in:

- `internal/scenario/data/fog_harbor.yaml`
- `internal/fogharbor`
- [Fog Harbor canon](08-fog-harbor-canon.md)

Its authoring paths are:

- `mainline`
- `flee`
- `dismissed`
