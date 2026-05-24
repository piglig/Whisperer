# Tools and Agent

## Purpose

The agent layer lets an LLM act as GM while deterministic tools own game state.
The model can narrate, ask for tool calls, and speak as NPCs, but it cannot
change rules or state without a tool.

## Packages

| Package | Purpose |
|---|---|
| `internal/agent` | provider adapters, GM loop, NPC agent, cassettes |
| `internal/orchestrator/tools` | tool definitions and handlers |
| `internal/orchestrator/sla` | validation of tool/narrative consistency |

## LLM Providers

Supported providers:

- Anthropic
- OpenRouter
- OpenAI
- Grok / xAI
- Gemini

Provider selection happens in CLI/config code. `internal/agent` exposes provider
interfaces and concrete adapters.

## GM Tool Loop

The GM loop:

1. sends the user turn and system prompt to the model
2. executes requested tool calls through a handler
3. sends tool results back to the model
4. repeats until final narrative or iteration limit
5. returns a `TurnTrace`

`TurnTrace` records:

- narrative
- tool calls
- token counts
- cost estimate
- truncation / iteration metadata

## Tool Boundaries

Tools are the only path for runtime state changes. They bridge:

- `internal/rules`
- `internal/store`
- `internal/memory`
- `internal/scenario`

Tool handlers must:

- validate input
- return structured output
- mark errors explicitly
- avoid leaking secrets
- leave enough trace detail for replay and SLA checks

## NPC Agent

NPC speech is delegated through `npc_speak`. The NPC agent receives persona,
secret, current context, and relevant memory. This keeps NPC voice and knowledge
boundaries more stable than asking the GM to improvise every line alone.

## SLA Checks

The orchestrator validates the GM output after tool execution.

Structural checks include:

- conclusions requiring rolls must have roll tools
- state changes must be tool-backed
- destroyed items must not reappear
- failed rolls must not be narrated as success
- forced endings must be honored

Optional judge checks use a helper model for:

- NPC consistency
- player knowledge projection
- ambiguous failed-roll contradiction

Judge failures are recorded in the SLA report and replay trace.

## Cassettes

Provider integration tests use go-vcr cassettes. Default test mode is
replay-only and does not require network access.

Refresh cassettes only when provider behavior intentionally changes:

```bash
make record-cassettes
```
