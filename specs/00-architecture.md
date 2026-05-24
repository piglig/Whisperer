# 00 — 总体架构

## Goals

- 给出 `Whisperer` 仓库的模块边界、依赖方向、回合管线与 SLA 校验切入点
- 作为后续模块 spec（01-rules、02-store、…）的引用基准
- 让任意一名贡献者只读这一篇就能定位到自己负责的包

## Non-Goals

- 不写算法细节（在 01-rules 等子 spec 中）
- 不写 prompt 内容（在 03-tools-and-agent 中）
- 不写剧本数据格式（在 05-scenario-and-triggers 中）

## 模块布局

工程按三条主链路组织，其他实验入口要么合并进这三条链路，要么移动到测试工具，要么删除：

- **Runtime**：玩家游玩链路，`cmd/whisperer` 默认 TUI → `internal/orchestrator` → tools → `store/rules/scenario/memory`
- **Authoring**：作者验收链路，`whisperer scenario lint|playtest|verify` → `internal/authoring` adapter registry → `internal/scenario` / `internal/director` / 剧本专属验收适配器
- **Observability**：复盘调试链路，trace JSONL → `internal/replay` → `whisperer replay` text / HTML viewer / exported playtest script

```
Whisperer/
├── cmd/whisperer/                 入口
├── internal/
│   ├── rules/                     纯函数规则引擎（W1）
│   ├── store/                     SQLite + sqlc 状态层（W2）
│   ├── memory/                    chromem-go 向量检索（W4）
│   ├── agent/                     anthropic-sdk-go 封装（W3）
│   ├── orchestrator/              回合循环 + tools + SLA + drift
│   ├── scenario/                  剧本 YAML 加载 + 触发器
│   └── tui/                       bubbletea（W6）
├── specs/                         本目录
└── runs/                          运行时日志（gitignore）
```

Runtime 依赖单向：`tui → orchestrator → tools → {agent, store, memory, scenario} → rules`。
`rules` 不引用任何其他 internal 包。

## 一回合（Turn）管线

```
TurnInput
  → IntentClassifier (Haiku)
  → PreTurnAutosaveHook
  → GMAgent.Respond(ctx, ...)            // tool_use 循环
       ├── tool: roll_skill / ...        → rules + 写 events
       ├── tool: get_npc_info / ...      → store 读
       └── tool: update_npc / ...        → store 写（turn-scoped Tx）
  → SLAValidator.Check(turnTrace)
       ├── 结构化失败 → 回滚 Tx，注入违规说明，重生成（最多 N=2 次）
       └── 关键回合追加 LLM judge
  → Memory.UpsertEvents(turnTrace)       // 异步
  → DriftDetector.Tick()                 // 软引导/硬收束
  → TurnResult{ Narrative, StateDiff, DiceLog }
```

## SLA 校验策略（混合）

需求文档 §11 八条 SLA：

| 项 | 方式 | 备注 |
|---|---|---|
| 1. 检定即工具 | 结构化 | `roll_*` tool_use 必须存在 |
| 2. 状态即工具 | 结构化 | `update_*` / `mark_*` / `transition_*` 必须存在 |
| 3. NPC 一致性 | LLM judge（按需） | 仅本回合涉及 NPC 长对话时触发 |
| 4. 物品守恒 | 结构化 | `destroyed_items` 集合反向匹配 |
| 5. 数值不采信 | 结构化 | 玩家文本中数值正则提取 + 反向匹配 |
| 6. 失败即失败 | 结构化（关键词） | 失败检定 → 文本不得含成功语义词 |
| 7. 角色知识闭环 | LLM judge（按需） | "你想起 / 你认出"投射句触发 |
| 8. 失败收束阻断 | 结构化 | hp/san 触阈 → 强制路由结局页 |

重生成协议：违规 → 回滚 Tx → 注入 `<sla_violation>` → 重生成（最多 2 次）→ 仍失败 → 模板兜底叙事 + 真实掷骰结果直出。

## 持久化分工

| 数据 | 存储 | 写入时机 |
|---|---|---|
| 实体状态 | SQLite | tool 调用同步 |
| 事件流水 | SQLite events 表 | 每 tool + 每回合 |
| 事件向量 | chromem-go 持久化目录 | 回合结束异步 |
| 存档元数据 | SQLite saves 表 | autosave + 启动时 `--save` |
| LLM 调用日志 | JSONL `runs/<save_id>/turns.jsonl` | 每回合 |

存档由 `save_id` 关联三处持久化资源。

## 关键不变量

1. `internal/rules` 不 import 任何兄弟包
2. `internal/agent` 不直接读写 store；走 `orchestrator/tools.Dispatcher`
3. `internal/tui` 不直接调 LLM；只调 `orchestrator.RunTurn(ctx, input)`
4. tool 调用必须在 turn-scoped Tx 内；SLA 失败即回滚整 Tx
5. 所有掷骰结果由代码生成、不可由 LLM 编造（接 SLA 1）

## 技术栈

见仓库根 `llm-rpg-cheerful-moonbeam.md` 与本目录 `adr/0001-go-instead-of-python.md`。
