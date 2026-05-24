# Quality, Release, and Maintenance

## Purpose

This document defines the quality bar for changes that affect runtime,
authoring, observability, or public documentation.

## Quality Gates

Baseline commands:

```bash
go test ./...
go build ./...
```

Recommended commands before larger PRs:

```bash
make test
make cover
make lint
```

Authoring validation:

```bash
go run ./cmd/whisperer scenario verify --scenario fog_harbor
```

Observability validation:

```bash
go run ./cmd/whisperer replay -h
```

## Coverage

Coverage is a guardrail, not a substitute for behavior tests. Keep tests focused
on user-visible behavior and core contracts.

High-risk areas:

- rules boundaries
- store migrations
- action guard decisions
- SLA rollback and retry
- scenario triggers
- authoring gates
- trace/replay schema
- secret redaction

## Documentation Requirements

Update documentation when a change affects:

- CLI behavior
- config keys
- trace shape
- scenario YAML shape
- authoring adapter behavior
- security posture
- release process

Use README for user-facing workflows, `specs/` for architecture references, and
ADRs for decisions that should not be relitigated.

## Release Notes

`CHANGELOG.md` follows Keep a Changelog.

Use:

- `Added` for new capabilities
- `Changed` for behavior changes
- `Deprecated` for functionality scheduled for removal
- `Removed` for deleted functionality
- `Fixed` for bugs
- `Security` for vulnerabilities

## Compatibility

The project is pre-1.0 and intentionally does not guarantee compatibility.
Prefer removing legacy code over preserving obsolete paths.

When breaking behavior, document:

- CLI flag changes
- config changes
- save or trace schema changes
- scenario YAML changes

## Security Checks

Before merging, ensure:

- no API keys in config, logs, traces, or cassettes
- cassettes have sensitive headers redacted
- path inputs are validated
- replay HTML embeds JSON safely
- user-provided scenario content cannot bypass tool boundaries

## Release Checklist

1. Run tests and build.
2. Run scenario verify.
3. Refresh documentation.
4. Update changelog.
5. Tag release.
6. Publish binaries when packaging is available.
