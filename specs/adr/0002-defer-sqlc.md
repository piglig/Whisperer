# ADR 0002: Use sqlc Behind the Store Repository

## Status

Accepted. Supersedes the earlier development-stage decision to defer sqlc.

## Context

The project originally used hand-written SQL through `database/sql` while the
schema was changing rapidly. That kept iteration fast, but the repository grew
enough that repeated `Scan`, null handling, and boolean conversion became a
maintenance cost.

## Decision

Use sqlc for store table queries.

Rules:

- SQL lives in `internal/store/queries`.
- generated code lives in `internal/store/storesqlc`.
- `make sqlc` regenerates code.
- external packages still use `internal/store.Repository`.
- generated types do not leak into runtime, authoring, or TUI packages.

## Consequences

Benefits:

- less hand-written boilerplate
- stronger compile-time coverage of query shapes
- easier review of SQL changes

Tradeoffs:

- generated code adds repository size
- schema changes require regeneration
- developers need sqlc available through the Makefile target

The repository boundary keeps this tradeoff local to `internal/store`.
