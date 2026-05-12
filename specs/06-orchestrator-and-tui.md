# 06 — Orchestrator & TUI（W6）

## Goals

- 把 W1-W5 全部组件串成一个 `Orchestrator.RunTurn(ctx, input) → TurnResult`
- 引入结构化 SLA 校验器（W3 推迟的 4 条最关键 SLA：#1 #4 #6 #8）
- 用 bubbletea 做最小可用 TUI：叙事流 + 状态条 + 命令行
- 接通真实 Anthropic API 路径，让 `whisperer` CLI 单二进制即可启动一局

## Non-Goals

- 不做 `/redo`、`/hint`：复杂度高、对验收路径不直接（推迟到 W7 抛光）
- 不做骰子动画、富文本布局：先把"能跑"做完
- 不做 LLM-as-judge SLA（#3 #7）：MVP 仅结构化，留接口
- 不做异步 RAG 召回；当前在 prompt 渲染前同步取 top-K

## Orchestrator

```go
package orchestrator

type Config struct {
    Store    *store.Store
    Memory   *memory.Memory
    Scenario *scenario.Scenario
    LLMGM    agent.LLM     // 主 LLM，必填
    LLMNPC   agent.LLM     // NPC 子代理；可与 GM 共享 client，模型常量不同
    SaveID   string
    RNG      *rand.Rand
    AutosaveEvery int      // 每 N 个回合做一次"软"快照（更新 saves.updated_at）
}

type Orchestrator struct { /* 持有 config + Engine + Detector + SLAValidator */ }

func New(cfg Config) (*Orchestrator, error)

type TurnResult struct {
    Narrative   string
    Trace       agent.TurnTrace
    Fired       []scenario.FiredTrigger
    Drift       scenario.DriftStatus
    Ending      *scenario.Ending  // 若已收束
    SLAReport   sla.Report
    Save        store.Save        // 回合结束后的最新存档元数据
}

func (o *Orchestrator) RunTurn(ctx context.Context, userInput string) (TurnResult, error)
```

### RunTurn 管线

```
1. snapshot pre-turn（save / investigator / location 摘要） → 渲染 system prompt
2. open turn-scoped Tx (store.RunTurn)
   2a. 构造 Dispatcher（tx-scoped repo, turn=currentTurn+1, rng, memory, npcAgent）
   2b. 构造 GMAgent（rendered system prompt, dispatcher.Tools(), dispatcher.Dispatch）
   2c. 第 1 次 agent.Respond(history, userInput) → trace
   2d. SLA.Check(trace, dispatcher 写入快照) → 若违规且 retries 剩余:
       - 内部不真正 rollback Tx（违规重生成在循环中）
       - 把 <sla_violation> 注入 system，再次 Respond
       - 最多重试 N=2，仍失败 → 标记 fallback 并继续（不阻断）
   2e. Engine.Evaluate（在同一 Tx 中触发剧本动作）
   2f. Drift.Tick（基于本回合 events）
   2g. CheckEndings
3. commit Tx（无论 SLA 是否最终降级）
4. AppendEvent: type=narrative，描述本回合 GM 文本（save_id 对外）
5. 异步 memory.UpsertEvent（不阻塞回合返回）
6. autosave: 每 AutosaveEvery 回合或在 Ending != nil 时立刻
```

注意：

- SLA 重试时不 rollback —— 因为 dispatcher 的工具调用已经按 LLM 意图执行
  状态变更，回滚会让 LLM 在重试时看不到已生成的真实数据。重试只是让 LLM
  读到 `<sla_violation>` 后更正叙事文本。结构化 SLA 失败更多是文本-状态不
  一致问题，文本可重写。
- 真正需要 hard rollback 的场景（store 写入失败 / Engine.Evaluate 报错）
  通过 RunTurn 返回 error 触发 Tx rollback。

## SLA 4 条（结构化）

```go
package sla

type Violation struct {
    Code        string
    Message     string
    Suggestion  string
}

type Report struct {
    Violations []Violation
    Passed     bool
}

type Validator struct { /* 持有 destroyedItemNames、investigator 状态等 snapshot */ }

func New(snap Snapshot) *Validator
func (v *Validator) Check(trace agent.TurnTrace) Report
```

### #1 检定即工具

- 在 narrative 中匹配关键词集合 `{"成功", "失败", "击中", "击退", "答应", "被说服", "通过检定", "未通过"}`
- 命中 → 检查 `trace.ToolCalls` 中是否存在任一 `roll_skill / roll_damage / sanity_check / opposed_roll`
- 不存在 → 违规（"narrative 出现检定结论但无对应 roll_* tool"）

### #4 物品守恒

- snapshot 中预先取 `store.ListDestroyedItems` 得到名字集合
- narrative 中出现任一已销毁物品名 → 违规

### #6 失败即失败

- 遍历 `trace.ToolCalls` 中失败的 roll_skill：解析 output 找 `success: false`
- 在该 tool 的 narrative 段后查找成功语义关键词 `{"成功", "答应", "信任你"}` —— MVP 用整段 narrative 的简化版
- 命中 → 违规

### #8 失败收束阻断

- snapshot 取 active investigator；如已 deactivate 或 hp/san=0 → 设置 `EndingForced = true`
- orchestrator 据此立即路由到 ending 页

## TUI

```
┌───────────────────────────────────────────────────────┐
│ Whisperer · Fog Harbor · Turn 5 · 黄昏                │  ← 状态条
├───────────────────────────────────────────────────────┤
│                                                       │
│  [GM] 灰雾在码头边缓缓翻涌……                          │
│                                                       │
│  [玩家] 我去酒馆找老板娘。                            │
│                                                       │
│  [GM] 玛丽莎抬起眼……                                  │
│                                                       │
├───────────────────────────────────────────────────────┤
│ HP 12 · MP 10 · SAN 60 · @ 雾港码头                   │  ← 侧边状态行
├───────────────────────────────────────────────────────┤
│ > _                                                   │  ← 输入
└───────────────────────────────────────────────────────┘
```

bubbletea 的 Model：

```go
type Model struct {
    orch     *orchestrator.Orchestrator
    save     store.Save
    inv      store.Investigator
    location store.Location
    log      []logEntry      // narrative + tool 调用提示
    input    textinput.Model
    spinner  spinner.Model
    busy     bool            // 等待 RunTurn
    err      error
    endingShown bool
    width, height int
    historyHint []string     // 上下方向键浏览
}
```

斜杠命令：

| 命令 | 行为 |
|---|---|
| `/save <name>` | 调 `orch.SaveAs`（重命名 + commit autosave） |
| `/load <name>` | 推迟到 W7：MVP 只支持启动时 `--save` |
| `/sheet` | 渲染调查员属性/技能 |
| `/inventory` | 渲染背包 |
| `/quit` | 退出 |
| `/talk <NPC>` | 在用户输入前加 prefix `[talk:NPC]` 让 GM 优先该 NPC |
| `/all` | 在用户输入前加 prefix `[all]` 表示对全场说 |
| `/time` | 显示当前时间段 |

异步：用户回车 → `tea.Cmd` 调 `orch.RunTurn`，完成后通过自定义 `turnDoneMsg` 回到 `Update`。

## main.go 接线

```
flags:
  --db <path>        SQLite 文件（默认 ./whisperer.db）
  --memory <dir>     memory 持久化目录（默认 ./mem）
  --scenario <id>    剧本 id（默认 fog_harbor）
  --save <name>      载入指定存档；缺省自动 new
  --api-key <key>    Anthropic API key（缺省读 ANTHROPIC_API_KEY 环境变量）
```

启动流程：
1. 解析 flags
2. `store.Open(db)` + `memory.New(memory)`
3. `scenario.LoadBundled(scenario)`
4. 初始化或载入 save：缺存档 → 创建 + 默认调查员（占位）+ Engine.Apply
5. 构造 LLM client（共享 anthropic.Client）
6. 构造 Orchestrator
7. 启动 bubbletea 程序

## 测试

- `orchestrator_test.go`：用 fakeLLM 跑一个完整 RunTurn，验证 trace、events、drift、autosave
- `sla_test.go`：4 条 SLA 各正/反例
- `tui_test.go`：Model.Update 对斜杠命令、回车、resize 的响应；不真正启动终端

覆盖率门槛 ≥ 85%。

## Open Questions

- ⏳ TUI `/load` 跨进程切 save 的复杂度；MVP 仅 CLI 起 / 单 save
- ⏳ 多调查员 / 绑定新调查员（需求文档 F6.4）实现路径——orchestrator 增加 `BindNewInvestigator(saveID, inv)` API，TUI 在 ending 页提供选项；W6 之后第一个 follow-up
