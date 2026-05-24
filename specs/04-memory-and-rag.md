# Memory and Retrieval

## Purpose

`internal/memory` provides semantic recall for events, NPC profiles, and clues.
It supports long-running investigations without forcing the prompt to contain
the entire history.

## Collections

Each save owns separate memory data for:

| Collection | Contents |
|---|---|
| events | turn summaries, major actions, discoveries |
| NPCs | persona, relationship, known state |
| clues | discovered clue text and metadata |

Memory is scoped by save ID. Cross-save continuity is modeled separately by
scenario meta state.

## Embedders

Supported modes:

| Mode | Use |
|---|---|
| `openai` | OpenAI embeddings |
| `openai_compat` | compatible endpoints such as Voyage |
| `fake` | deterministic local tests |
| `off` | disable semantic memory |

API keys are read from environment variables or CLI flags. They must not be
stored in config files.

## Runtime Flow

Writes:

- scenario initialization can seed NPC and clue memory
- turn completion can upsert event memory
- clue discovery can upsert clue memory
- major NPC changes can refresh NPC memory

Reads:

- orchestrator retrieves relevant events before GM prompt rendering
- NPC tool calls retrieve NPC-specific context
- clue and event recall can inform GM narration without overriding rules

## Constraints

- memory retrieval is advisory; store state remains authoritative
- fake embedder must be deterministic
- memory failures should not corrupt store state
- secrets must be redacted before any shareable trace is written

## Testing

Memory tests should cover:

- collection creation
- upsert and query behavior
- fake embedder determinism
- persistence when a directory is provided
- disabled/off behavior through runtime configuration
