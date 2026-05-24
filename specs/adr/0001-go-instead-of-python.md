# ADR 0001: Use Go for the Runtime

## Status

Accepted.

## Context

Whisperer needs a deterministic rules engine, local persistence, a terminal UI,
LLM tool-use orchestration, and a single-command developer workflow. The initial
product idea could have been implemented in Python because Python has a large
LLM ecosystem.

The project prioritizes:

- static contracts around game state and rule results
- a local single-binary distribution path
- simple concurrency and cancellation
- strong testability without network access
- clear package boundaries

## Decision

Implement Whisperer in Go.

Primary libraries:

- Bubble Tea / Lip Gloss for TUI
- official provider SDKs for LLM access
- modernc SQLite for local persistence
- sqlc for generated store queries
- chromem-go for embedded vector memory
- go-vcr for provider integration cassettes
- `log/slog` for structured logging

## Consequences

Benefits:

- deterministic rules and trace structs are type-safe
- packaging can become a single binary
- store, rules, authoring, and replay contracts are easier to test
- fewer framework-level abstractions hide the agent/tool boundary

Tradeoffs:

- less off-the-shelf LLM orchestration than Python ecosystems
- more custom prompt, tool, and retrieval plumbing
- fewer mature high-level RAG utilities

The tradeoff is intentional: Whisperer is primarily an architecture and product
systems project, not a wrapper around an existing agent framework.
