# 03 — Tools & GM Agent（W3）

## Goals

- 用 `anthropic-sdk-go` 把 GM 跑成一个真实的 tool-use agent
- 提供 ~15 个 tool（rules 类、读类、写类），所有状态变更必须经 tool（接 SLA #1 #2）
- agent 与 tool 解耦：agent 只持有 `LLM` 接口与一个 `ToolHandler` 函数；tool 定义和派发实现在 orchestrator 层
- 用 fake LLM 即可单测，不需要真实 API 调用

## Non-Goals

- W3 不接入 SLA 校验器（W4 之后）
- W3 不接入 RAG 检索（W4 memory 包）
- W3 不接入剧本触发器（W5）
- W3 不实装 prompt caching（实现后留 TODO，W4 同步加上）—— 见 ADR 0003

## 包结构

```
internal/agent/                LLM 抽象 + GM 主循环（不知道具体 tool）
├── client.go                  LLM interface + anthropic 适配器 + 模型常量
├── gm.go                      GMAgent.Respond tool_use 循环
├── trace.go                   TurnTrace（文本 + 工具调用记录，供 SLA 用）
└── prompts/
    └── gm_system.tmpl         系统 prompt（embed）

internal/orchestrator/tools/   Tool 定义 + Dispatcher（桥接 rules / store）
├── dispatcher.go              Dispatcher 结构 + 公共类型
├── definitions.go             所有 ToolParam 定义（Schema）
├── handlers_rules.go          roll_skill / roll_damage / sanity_check / opposed_roll
├── handlers_read.go           get_* / list_*
├── handlers_write.go          update_* / mark_* / move_* / destroy_* / transition_* / add_event
└── errors.go                  tool 错误统一 ToolError 格式
```

## LLM 抽象

```go
package agent

type LLM interface {
    NewMessage(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error)
}

// 真实实现
type Anthropic struct{ client anthropic.Client }
func NewAnthropic(apiKey string) *Anthropic
func (a *Anthropic) NewMessage(ctx, params) (*anthropic.Message, error)

// 模型常量
const (
    ModelGM     = anthropic.ModelClaudeSonnet4_5_20250929 // TODO: 4.6 / 4.7
    ModelHelper = anthropic.ModelClaudeHaiku4_5
)
```

## GM 主循环

```go
type ToolHandler func(ctx context.Context, name string, inputRaw []byte) (output any, isError bool)

type GMAgent struct {
    llm     LLM
    model   anthropic.Model
    system  string
    tools   []anthropic.ToolUnionParam
    handler ToolHandler
    maxIter int    // tool_use 循环上限，防御性
}

func (g *GMAgent) Respond(
    ctx context.Context,
    history []anthropic.MessageParam,  // 上文消息（不含本回合 user input）
    userInput string,
) (TurnTrace, []anthropic.MessageParam, error)
```

返回值：
- `TurnTrace` 是本回合的累积记录（见下）
- 第二个返回值是更新后的 history（含本回合的 user input、assistant 回复、所有 tool_use/tool_result），调用方可保留作为下一回合输入

循环逻辑严格按 SDK 示例：
1. 用 `messages` 调一次 `Messages.New`
2. 遍历 `message.Content`：累积 TextBlock 到 `trace.Narrative`，记录 ToolUseBlock 到 `trace.ToolCalls`
3. `messages = append(messages, message.ToParam())`
4. 对每个 ToolUseBlock 调 `handler`，把结果以 `anthropic.NewToolResultBlock(blockID, jsonStr, isError)` 加到 `toolResults`
5. 没有 tool_use → 退出；否则把 toolResults 包成 `NewUserMessage` 追加，下一轮
6. 超过 `maxIter` 强制退出并在 trace 标记 `Truncated=true`

## TurnTrace

```go
type ToolCall struct {
    ID       string          `json:"id"`         // anthropic 给的 block.ID
    Name     string          `json:"name"`
    Input    json.RawMessage `json:"input"`      // 原始 input
    Output   json.RawMessage `json:"output"`     // handler 返回的 JSON
    IsError  bool            `json:"is_error"`
    Iter     int             `json:"iter"`       // 第几个 LLM 回合产生的
}

type TurnTrace struct {
    Narrative   string     `json:"narrative"`     // 所有 TextBlock 拼接
    ToolCalls   []ToolCall `json:"tool_calls"`
    Iterations  int        `json:"iterations"`
    Truncated   bool       `json:"truncated"`
    InputTokens  int       `json:"input_tokens"`  // 累计
    OutputTokens int       `json:"output_tokens"`
}
```

## Tool 集（W3 MVP）

| 名称 | 类别 | 入参 schema 关键字段 | 返回 |
|---|---|---|---|
| `roll_skill` | rules | skill_name, skill_value, difficulty, bonus_dice, penalty_dice | `SkillCheckResult` |
| `roll_damage` | rules | expression | `DamageResult` |
| `sanity_check` | rules+state | investigator_id, loss_pass, loss_fail | `SanityResult`（同时落 SAN 到 store） |
| `opposed_roll` | rules | actor_name, actor_skill, target_name, target_skill | `OpposedResult` |
| `get_investigator` | read | (无参，取当前 active) | `Investigator` |
| `get_npc` | read | npc_id | `NPC` |
| `get_location` | read | location_id | `Location` |
| `list_npcs_at_location` | read | location_id | `[]NPC` |
| `list_found_clues` | read | (无参) | `[]Clue` |
| `update_investigator_vitals` | write | investigator_id, hp, mp, san | OK |
| `update_npc_relation` | write | npc_id, delta | OK |
| `kill_npc` | write | npc_id | OK |
| `mark_location_visited` | write | location_id | OK |
| `move_item` | write | item_id, owner_type, owner_id | OK |
| `destroy_item` | write | item_id | OK |
| `mark_clue_found` | write | clue_id, location_id, turn | OK |
| `transition_location` | write | location_id, turn | OK（同时更新 save 进度） |
| `add_event` | write | turn, type, description, related_entities | event_id |

约束：
- `save_id` / `investigator_id` 上下文由 Dispatcher 持有，不暴露给 LLM
- 所有 tool 失败 → 返回 `{"error": "..."}` 并设 `is_error=true`，让 LLM 看到错误后调整

## Dispatcher

```go
type Dispatcher struct {
    repo            *store.Repository
    saveID          string
    rng             *rand.Rand
    nowMS           func() int64           // 注入式时间
}

func New(repo *store.Repository, saveID string, rng *rand.Rand) *Dispatcher
func (d *Dispatcher) Tools() []anthropic.ToolUnionParam
func (d *Dispatcher) Dispatch(ctx context.Context, name string, inputRaw []byte) (any, bool)
```

`Dispatch` 内 switch 路由到具体 handler；handler 解析 input、调 rules / repo、组装输出结构。

## System Prompt（要点）

`prompts/gm_system.tmpl` 用 `text/template`，至少包含：
- 角色：CoC 7e GM；不主动剧透；尊重玩家自由意志
- 必须经 tool 才能宣判检定结果与状态变更（接 SLA #1 #2）
- NPC 一致性、角色知识投射要求
- 回合上下文：当前剧本 / 调查员状态摘要 / 当前地点 / 最近线索

W3 模板字段先放 placeholder（剧本字段在 W5 填）。

## 测试

不依赖真实 API：
- `fake_llm_test.go` 提供 `fakeLLM`，按预设脚本返回 `*anthropic.Message`
- `gm_test.go` 验证多轮 tool_use 循环、`Truncated` 边界
- `dispatcher_test.go` 对每个 tool 走桥接：解析 input → 检查 store / rules 副作用

覆盖率门槛 ≥ 85%（与现有标准一致）。

## Open Questions

- ⏳ Prompt caching：`MessageNewParams.System` 与 tools 的 `cache_control` 怎么用？W4 之前补 ADR + 实装
- ⏳ Token 预算：单回合上限？现先用 SDK 默认 `MaxTokens=4096`，记到 ADR 中
- ⏳ 模型常量先用 SDK 提供的最新（Sonnet 4.5），等 SDK 升级到 4.6/4.7 时跟进
