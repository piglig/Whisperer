# Fog Harbor Canon

## Audience

This is a GM-side reference for the bundled `fog_harbor` scenario. It contains
spoilers and should not be rendered directly to players.

## Premise

The investigator arrives in Fog Harbor to investigate the disappearance of Lucy
Mason, a 16-year-old local girl. The public story is that Lucy ran away. The
real story is a decades-old pact between town leaders and an entity beneath the
lighthouse reef.

## Truth

Thirty years ago, after a maritime disaster, a small circle of townspeople made
a pact with a reef-dwelling elder. Every few years, the town provides a socially
isolated victim. In exchange, Fog Harbor is spared storms, plague, and other
threats from the sea.

Lucy is the latest victim. Anna Rivers is at risk of becoming the next.

## Main Actors

| ID | Name | Role | Secret |
|---|---|---|---|
| `vance` | Dr. Vance | doctor, executor variant | manages sedation and victim selection |
| `marisa` | Marisa | pub owner | coerced record keeper |
| `helena` | Helena | Lucy's mother | grieving and unaware |
| `orin` | Orin | lighthouse keeper | paid lookout, now wavering |
| `rourke` | Rourke | police chief | bribed to bury evidence |
| `father_calvin` | Father Calvin | priest | pact believer and keeper of old records |
| `anna` | Anna Rivers | pub worker | next likely victim |
| `lucy` | Lucy Mason | missing girl | already sacrificed except in the best ending |
| `deep_elder` | Deep Elder | mythic antagonist | can be confronted or sealed |

## Variants

Variants rotate the current executor while preserving the same historical pact.

| Variant | Active executor |
|---|---|
| `vance_executes` | Dr. Vance |
| `calvin_directs` | Father Calvin |
| `rourke_runs` | Rourke |

## Clue Structure

Fog Harbor uses three clue tiers.

### Tier 1: Surface Disappearance

- `blood_letter`
- `tide_chart`
- `lucy_diary`
- `cloth_scrap`

### Tier 2: Human Conspiracy

- `ledger`
- `doctor_visits_log`
- `clinic_supplies`
- `rourke_bribe`
- `parish_record`
- `empty_graves`
- `anna_warning`

### Tier 3: Mythic Truth

- `strange_chant`
- `reef_carvings`
- `sacrifice_chamber`
- `deep_elder_sighting`

### Red Herring

- `marisa_exhusband`

## Key Conclusions

Each key conclusion has at least three clue routes.

| Conclusion | Supporting clues |
|---|---|
| Lucy did not run away | `lucy_diary`, `cloth_scrap`, `sacrifice_chamber` |
| the disappearances are patterned | `parish_record`, `empty_graves`, `tide_chart` |
| town authorities are involved | `ledger`, `doctor_visits_log`, `rourke_bribe` |
| the reef is mythic, not mundane | `strange_chant`, `reef_carvings`, `deep_elder_sighting` |

## Pressure

Anna gives the mystery a living stake. If the player delays too long without
confronting the culprit or building enough trust with Anna, the scenario can
trigger `anna_taken`.

## Endings

| Ending | Kind | Meaning |
|---|---|---|
| `pact_broken` | success | best ending, pact disrupted |
| `solved` | success | truth exposed with evidence |
| `flee_with_truth` | partial | investigator escapes with enough truth |
| `victim_dies` | failure | another victim is lost |
| `dismissed` | failure | investigator is pushed out |

System-level investigator death or indefinite insanity is handled by the
orchestrator, not scenario YAML.

## Authoring Coverage

Fog Harbor adapter paths:

- `mainline`: covers all variants and reaches a success ending
- `flee`: covers partial success
- `dismissed`: covers social failure

Verify gates include generic authoring gates plus expected ending coverage for
`solved`, `flee_with_truth`, and `dismissed`.
