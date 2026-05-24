# State Store

## Purpose

`internal/store` provides durable local game state. It wraps SQLite behind a
small repository API so runtime and authoring code do not depend on SQL details.

## Responsibilities

- manage schema migrations
- open and close SQLite databases
- provide typed CRUD and domain operations
- support turn-scoped transactions
- map missing rows to project errors
- keep generated SQL code behind the repository boundary

## Storage Model

Core tables:

| Table | Purpose |
|---|---|
| `saves` | scenario, variant, location, turn, stage, timestamps |
| `investigators` | player characters, vitals, attributes, skills |
| `npcs` | NPC state, relation, location, alive flag |
| `locations` | location graph and visited state |
| `items` | item ownership and destroyed state |
| `clues` | clue discovery state |
| `events` | append-only gameplay and system event stream |

## Repository Boundary

Consumers use:

```go
st, err := store.Open(ctx, "whisperer.db")
repo := st.Repo()
```

The repository owns:

- domain model conversion
- SQL null handling
- JSON field serialization
- `ErrNotFound` mapping
- transaction-scoped repository creation

Do not add direct SQL access outside `internal/store`.

## Transactions

Runtime turns execute in a transaction. If SLA validation fails during an
attempt, the attempt rolls back and the GM is asked to regenerate with feedback.

Authoring playtests use the same repository semantics against an in-memory
store.

## sqlc

Store queries are generated with sqlc:

```bash
make sqlc
```

Generated code lives in `internal/store/storesqlc`. The public repository API
should remain the only API used by other packages.

## Migrations

Migrations live in `internal/store/migrations`.

Rules:

- migrations must be deterministic
- migrations must be covered by tests
- do not mutate old migration files after they have shipped; add a new one
- development-stage breaking changes are acceptable, but keep the migration
  history understandable

## Security

- use parameterized queries only
- never write API keys to store tables
- treat traces and events as shareable debugging artifacts and redact secrets
  before persistence

## Test Expectations

Store changes should cover:

- migration from empty database
- CRUD behavior
- transaction rollback
- JSON field round trips
- event ordering
- foreign-key behavior
