# Contributing to Whisperer

Thank you for contributing to Whisperer. This project welcomes bug reports,
documentation fixes, scenario work, rules improvements, UX proposals, and
architecture cleanup.

The project is still in active development. Compatibility is not guaranteed, so
prefer clean designs over compatibility shims.

## Code of Conduct

All participation is governed by [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## Before You Start

Open an issue first unless the change is clearly mechanical, such as a typo,
small documentation correction, or narrow test fix.

Good issues describe:

- the player, author, or maintainer workflow affected
- the current behavior
- the desired behavior
- any relevant trace, log, or scenario snippet

## Development Environment

Required:

- Go 1.25.8 or newer
- Git

Optional:

- `golangci-lint`
- an LLM provider API key for e2e validation

Build and test:

```bash
make build
make test
make cover
make lint
```

Direct equivalents:

```bash
go build ./...
go test ./...
go test -race ./...
go vet ./...
```

## Architecture Lanes

Keep changes aligned to one of the three main lanes:

| Lane | Scope |
|---|---|
| Runtime | player TUI, orchestrator, tools, store, rules, scenario, memory |
| Authoring | scenario lint, playtest, verify, director, content gates |
| Observability | trace, replay, reports, e2e validation |

Code that does not belong to one of these lanes should either be test-only,
merged into an existing lane, or deleted.

## Pull Request Process

1. Branch from `main`.
2. Keep commits focused and buildable.
3. Add or update tests for behavior changes.
4. Update documentation for user-visible, authoring, or architecture changes.
5. Add an `[Unreleased]` changelog entry when the change is notable.
6. Fill out the PR template completely.

Commit message style:

- imperative mood: `add replay viewer export`
- first line no longer than 72 characters
- explain why in the body when the change is not obvious

## Testing Expectations

For most PRs:

```bash
go test ./...
go build ./...
```

For runtime or orchestrator changes:

```bash
go test ./internal/orchestrator ./internal/orchestrator/tools ./internal/tui
```

For authoring changes:

```bash
go test ./internal/authoring ./internal/fogharbor ./internal/scenario
go run ./cmd/whisperer scenario verify --scenario fog_harbor
```

For observability changes:

```bash
go test ./internal/replay ./cmd/whisperer
go run ./cmd/whisperer replay -h
```

For real LLM validation, use:

```bash
go run ./cmd/whisperer e2e --provider openai --input "我环顾四周"
```

## Scenario Contributions

New scenarios should include:

- YAML scenario data under `internal/scenario/data/`
- a canon/reference document under `specs/`
- an `internal/authoring.ScenarioAdapter`
- at least one deterministic playtest path
- a `scenario verify` gate that can run without LLM calls

Scenario quality requirements:

- every key conclusion has at least three independent clue paths
- blocked actions provide a reason and a recovery path
- variants preserve the same world truth unless the scenario explicitly models
  alternate reality
- endings are reachable through deterministic authoring tests

## Cassettes

`internal/agent` uses go-vcr cassettes for LLM provider integration tests.

Default test mode is replay-only and does not require network access or API
keys.

To refresh cassettes:

```bash
make record-cassettes
```

Only commit cassettes after checking that secrets are redacted.

## Dependencies

Prefer the Go standard library when it is sufficient. New dependencies should
be justified in the PR description and must be compatible with Apache-2.0
distribution.

## Security

Do not report vulnerabilities in public issues. Follow [SECURITY.md](SECURITY.md).

## License

By contributing, you agree that your contribution is licensed under
Apache-2.0.
