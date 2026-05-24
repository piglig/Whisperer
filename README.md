# Whisperer

Whisperer is a local, single-player investigative TRPG engine. An LLM plays the
GM, while Go code owns dice, skills, sanity, combat, state transitions, scenario
triggers, playtest verification, and replay debugging.

The current bundled scenario is **Fog Harbor** (`fog_harbor`), an original
Cthulhu-flavored mystery built around deterministic d100 mechanics.

> Whisperer is an unofficial fan project. *Call of Cthulhu* is owned by
> Chaosium Inc. This repository is not affiliated with or endorsed by Chaosium.
> It does not redistribute published Chaosium scenarios, art, or stat blocks.

## Highlights

- **Deterministic rules**: dice, HP, SAN, skills, clues, item movement, and
  endings are resolved by Go tools, not invented by the model.
- **Scenario authoring loop**: `scenario lint`, deterministic playtests, and
  content gates keep scenarios shippable.
- **Observability-first debugging**: every turn can be written to JSONL,
  replayed in text, exported to an interactive HTML viewer, or converted into a
  playtest script.
- **LLM guardrails**: structural SLA checks and optional LLM-as-judge checks
  catch missing tool calls, knowledge leaks, failed-roll contradictions, and
  forced ending conditions.
- **Replayability**: Fog Harbor ships with variants, multiple endings, staged
  clues, and cross-run meta memory.

## Project Status

Whisperer is under active development. The codebase intentionally does not
preserve compatibility while the core architecture is still being shaped.

Primary development lanes:

| Lane | Command surface | Purpose |
|---|---|---|
| Runtime | `whisperer` | Player-facing TUI game loop |
| Authoring | `whisperer scenario lint|playtest|verify` | Scenario validation and content gates |
| Observability | `whisperer e2e`, `whisperer replay` | LLM validation, trace review, reports |

## Requirements

- Go 1.25.8 or newer
- One supported LLM provider key for real gameplay
- Optional OpenAI-compatible embedding provider for semantic memory

Supported LLM providers:

- Anthropic
- OpenRouter
- OpenAI
- Grok / xAI
- Gemini

## Installation

Build from source:

```bash
git clone https://github.com/piglig/Whisperer.git
cd Whisperer
go build -o whisperer ./cmd/whisperer
./whisperer --help
```

The project also includes a first-run setup wizard:

```bash
./whisperer init
```

## Configuration

Whisperer reads configuration from:

- `$XDG_CONFIG_HOME/whisperer/config.toml`
- `$HOME/.config/whisperer/config.toml`
- `%APPDATA%/whisperer/config.toml` on Windows
- `--config /path/to/config.toml`

Precedence is:

```text
CLI flags > environment variables > config file > built-in defaults
```

API keys should be provided through environment variables, not config files:

```bash
export ANTHROPIC_API_KEY=sk-ant-...
export OPENROUTER_API_KEY=sk-or-...
export OPENAI_API_KEY=sk-...
export XAI_API_KEY=xai-...
export GEMINI_API_KEY=...
```

See [docs/example-config.toml](docs/example-config.toml) for a complete config
file with comments.

## Quick Start

Run the default TUI:

```bash
./whisperer
```

Choose a provider explicitly:

```bash
./whisperer --provider openai
./whisperer --provider openrouter
./whisperer --provider gemini
```

Create a deterministic run for debugging:

```bash
./whisperer --scenario fog_harbor --variant calvin_directs --seed 42
```

Use a different investigator template:

```bash
./whisperer --investigator-template private_eye
./whisperer --investigator-template doctor
```

Resume a save:

```bash
./whisperer --save <save-id>
```

## Runtime Controls

The TUI centers the case board and actionable choices. Numbered actions can be
selected directly, while free-form input remains available.

Common slash commands:

| Command | Description |
|---|---|
| `/talk <NPC>` | Address a specific NPC |
| `/all <text>` | Speak to everyone present |
| `/hint` | Request a non-spoiler nudge |
| `/bind <name> <occupation>` | Bind a replacement investigator after death |
| `/help` | Show in-game help |

Blocked actions are reported through a structured decision object containing a
reason code, player-facing explanation, debug reason, required clues, and
suggested recovery actions.

## Authoring Commands

Lint scenario structure:

```bash
./whisperer scenario lint --scenario fog_harbor
./whisperer scenario lint --all
./whisperer scenario lint --scenario fog_harbor --json
```

Run deterministic playtests:

```bash
./whisperer scenario playtest --scenario fog_harbor --path mainline
./whisperer scenario playtest --scenario fog_harbor --all
./whisperer scenario playtest --scenario fog_harbor --all --json
```

Run the full content gate:

```bash
./whisperer scenario verify --scenario fog_harbor
./whisperer scenario verify --scenario fog_harbor --json
```

Authoring adapters live behind `internal/authoring.ScenarioAdapter`. Adding a
new scenario should not require adding a new CLI command.

## Observability Commands

Run a real LLM e2e validation without launching the TUI:

```bash
./whisperer e2e --provider openai --input "我环顾码头四周"
./whisperer e2e \
  --inputs-file cmd/whisperer/e2e_scripts/fog_harbor_mainline.txt \
  --turns 25 \
  --enable-judge
```

Replay a trace:

```bash
./whisperer replay runs
./whisperer replay --turn 7 runs/<save-id>/<session>.jsonl
./whisperer replay --html tmp/replay.html runs/<save-id>/<session>.jsonl
./whisperer replay --export-script tmp/playtest.txt runs/<save-id>/<session>.jsonl
./whisperer replay --turn 7 --mark bad,misjudge --note "NPC contradicted a clue" runs/<save-id>/<session>.jsonl
```

Replay output includes player input, GM output, tool calls, SLA checks, judge
checks, state diff, endings, blocked-action explanations, and annotations.

## Semantic Memory and Judge

Enable OpenAI-compatible embeddings:

```bash
export OPENAI_API_KEY=sk-...
./whisperer --embedder openai --embedder-model text-embedding-3-small
```

Use another compatible endpoint:

```bash
export VOYAGE_API_KEY=...
./whisperer \
  --embedder openai_compat \
  --embedder-base-url https://api.voyageai.com/v1 \
  --embedder-model voyage-3-large \
  --embedder-api-key-env VOYAGE_API_KEY
```

Useful development modes:

```bash
./whisperer --embedder fake
./whisperer --embedder off
./whisperer --enable-judge
```

## Repository Layout

```text
Whisperer/
├── cmd/whisperer/          Runtime, Authoring, and Observability CLI
├── docs/                   User-facing examples
├── internal/
│   ├── agent/              LLM providers, GM/NPC agents, cassettes
│   ├── authoring/          Scenario adapter registry, playtests, gates
│   ├── config/             TOML/env/flag configuration
│   ├── director/           Scenario pacing and advice
│   ├── fogharbor/          Fog Harbor adapter and path scripts
│   ├── memory/             Semantic memory and embedders
│   ├── orchestrator/       Turn loop, action decisions, SLA, tools
│   ├── replay/             Trace document model and HTML viewer
│   ├── rules/              Pure d100 rules
│   ├── scenario/           YAML loader, variants, triggers, reports
│   ├── store/              SQLite persistence
│   └── tui/                Bubble Tea terminal UI
├── specs/                  Architecture, subsystem references, ADRs
└── runs/                   Local traces and meta state, gitignored
```

## Development

```bash
make build
make test
make cover
make lint
```

Equivalent direct commands:

```bash
go build ./...
go test ./...
go test -race ./...
go vet ./...
```

## Documentation

- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)
- [Changelog](CHANGELOG.md)
- [Architecture overview](specs/00-architecture.md)
- [Scenario authoring](specs/05-scenario-and-triggers.md)
- [Fog Harbor canon](specs/08-fog-harbor-canon.md)
- [Original product brief](llm-rpg-cheerful-moonbeam.md)

## License

Apache-2.0. See [LICENSE](LICENSE).
