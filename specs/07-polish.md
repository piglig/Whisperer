# 07 — Polish（W7）

## Goals

W7 是 MVP 之上的抛光：把第 §F 系列里 W6 推迟的小项收口，并补足产品级体验细节。

涵盖项：

1. **Impale 完整规则**（rules.combat）：穿刺武器 extreme/critical 触发标准 impale；非穿刺武器即使 extreme 也按普通伤害
2. **F6.4 多调查员绑定**：`orchestrator.BindNewInvestigator` + TUI `/bind` 命令；旧调查员 deactivate 但行存档保留，剧本世界状态保留
3. **`/hint` 命令**：玩家求助 → GM 仅给环境/NPC 暗示，不直接给答案
4. **autosave checkpoint 事件**：关键 tool 后写 `events.type=autosave_checkpoint`，标签包含场景切换 / SAN 归零 / NPC 死亡 / 不定性疯狂
5. **LLM-as-judge**：可选 Judge 接口接入 SLA #3 NPC 一致性 + #7 角色知识闭环；Haiku 实现，默认关闭
6. **README**：项目介绍 + 架构 + 运行说明 + 限制清单

## Non-Goals

- `/redo` 完整实现（涉及 history rewind 与 Tx 回放，单独 milestone）
- autosave 的"回滚到 checkpoint" 动作（事件标记 + 文件克隆是不同事项）
- F1.4 全 24 职业模板分步建卡（仍为快速生成 + 占位调查员；建卡向导留待后续）

## 详细变更

### 1. Impale

`rules.Combatant` 新增 `Impaling bool`：

```go
type Combatant struct {
    ...
    Impaling bool
}
```

`Attack` 仅在 `attacker.Impaling` 为真且 degree ∈ {extreme, critical} 时调用 `applyImpale`，否则按普通伤害。`applyImpaleApprox` 改名为 `applyImpale` 并修订注释。

### 2. F6.4 BindNewInvestigator

```go
func (o *Orchestrator) BindNewInvestigator(ctx context.Context, inv store.Investigator) error
```

- 把所有 active 调查员 deactivate（保留行）
- upsert 新调查员并 active=true
- 清空 history，避免下回合 prompt 带"前任"的对话遗留
- 不重置剧本世界状态：NPC、地点、线索、事件流水保留

TUI 新增 `Binder` 接口（Runner 的可选扩展）；Model 在 `endingShown` 后接受 `/bind <name> <职业>`，用默认属性快速建卡。完整建卡向导（F1.4）留给后续。

### 3. /hint

TUI 命令 `/hint`：把 user input 改为 `[hint] ...` 系统提示语，让 GM 用环境/NPC 暗示拉回主线。GM system prompt（gm_system.tmpl §Granularity）已包含相关要求；这里通过显式输入触发暗示。

不引入新的后端 tool —— 复用现有 add_event / npc_speak 等。

### 4. Autosave checkpoint

`store.EventType` 新增 `EventAutosaveCheckpoint = "autosave_checkpoint"`。

`tools/dispatcher.go` 暴露 `markAutosave(ctx, d, label)`。在以下 handler 成功执行后调用：

| Tool | label |
|---|---|
| `transition_location` | `scene_transition:<location_id>` |
| `kill_npc` | `npc_killed:<npc_id>` |
| `update_investigator_vitals`（hp ≤ 0 或 san ≤ 0 时） | `investigator_lost:<id>` |
| `sanity_check`（不定性疯狂 / SAN 归零） | `indefinite_insanity:<id>` 或 `san_zero:<id>` |

事件流水仅作为标记 + UI 复盘用；本里程碑不实现"回滚到 checkpoint"。

### 5. LLM-as-judge

`internal/orchestrator/sla/judge.go`：

```go
type Judge interface {
    JudgeNPCConsistency(ctx, JudgeNPCInput) (*Violation, error)
    JudgeKnowledgeProjection(ctx, JudgeKnowledgeInput) (*Violation, error)
}

func (v *Validator) CheckWithJudge(ctx, trace, judge, JudgeContext) Report
type HaikuJudge struct { ... }
```

- `CheckWithJudge` 先跑结构化 Check，再按 trace 中是否含 `npc_speak` 或 narrative 含"你想起 / 你认出"等关键词决定是否调 Judge
- `HaikuJudge` 是基于 Haiku 的最小实现：单次 prompt 要求 LLM 输出严格 `{"violation":bool,"reason":""}` JSON；解析失败视为 pass，避免误伤
- orchestrator `Config.Judge sla.Judge` 默认 nil；启用方需要显式注入（CLI 之后可加 `--enable-judge` flag）
- 错误被吞：judge 失败不会让 SLA Report 失败

代价：每个回合最多 +2 次 Haiku 调用。建议仅在 NPC 频繁出场或调查员能力差异大时启用。

### 6. README

放在仓库根 `README.md`，包含：
- 1-2 段项目介绍 + 与 AI Dungeon 的差异
- 架构图（复用 specs/00-architecture.md 的简化版）
- 运行说明：依赖、go build、ANTHROPIC_API_KEY、`scenario verify`、`e2e --enable-judge`
- 已实现 / 未实现清单（与需求文档 Goals/Non-Goals 对齐）

## 测试

新增覆盖：
- `rules/combat_test.go`：impale on / off 两条路径
- `orchestrator/orchestrator_test.go`：BindNewInvestigator 多调查员场景
- `orchestrator/tools/handlers_*`：autosave 事件检查（间接通过 events 表）
- `orchestrator/sla/judge_test.go`：fakeJudge + HaikuJudge with fake LLM

整体覆盖率门槛保持 ≥ 85%。

## Open Questions

- ⏳ `/redo` 历史回滚的实现路径：基于 LLM 历史还是基于 events 流水？建议 W8 单独评估
- ⏳ Autosave 文件级回滚 vs 单 turn 回滚：前者更稳但占空间，后者依赖 Tx 重放
- ⏳ Judge 误报率与成本：积累 50+ 真实回合数据后再决定默认是否开启
