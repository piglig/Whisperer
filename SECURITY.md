# Security Policy

## Supported Versions

Whisperer is a pre-1.0 project. Security fixes target the latest development
line only.

| Version | Supported |
|---|---|
| latest `main` / latest release | Yes |
| older development snapshots | No |

## Reporting a Vulnerability

Do not open a public issue for security vulnerabilities.

Use GitHub private vulnerability reporting from the repository Security tab:

1. Open the repository on GitHub.
2. Select **Security**.
3. Choose **Report a vulnerability**.
4. Include reproduction steps and impact.

Expected response:

- acknowledgement within 3 business days
- initial assessment within 14 days
- coordinated fix and disclosure timeline when the report is valid

## What to Include

Please include:

- affected component, package, command, or file format
- affected version or commit
- reproduction steps
- impact assessment
- logs or traces with secrets removed
- suggested mitigation, if available

## Threat Model

Whisperer is currently a local, single-user CLI/TUI application. Security work
focuses on:

- API key exposure in logs, traces, cassettes, error messages, and config files
- prompt injection through scenario YAML, user input, NPC knowledge, and tool
  arguments
- path traversal in current or future user-provided scenario loading
- unsafe deserialization or SQL injection
- denial of service through unbounded trace, replay, or scenario inputs
- dependency vulnerabilities

## Existing Safeguards

- API keys are expected through environment variables or CLI flags, not config.
- `internal/secrets` redacts known key and bearer-token shapes before trace
  persistence.
- structured logging masks sensitive attribute names.
- cassette tests redact sensitive headers.
- SQLite access goes through typed repository methods and parameterized queries.

## Out of Scope

The following are not handled as private security reports unless they expose a
separate vulnerability:

- ordinary LLM hallucinations
- unexpected story outcomes
- user-controlled API spend
- terminal rendering glitches
- spoilers from local files intentionally opened by the user

## Recognition

Valid reporters may be credited in release notes unless they request otherwise.
