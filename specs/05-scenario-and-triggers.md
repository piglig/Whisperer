# 05 — Scenario & Triggers（W5）

## Goals

- 用 YAML 描述剧本：场景列表、NPC、线索、触发器、主线节点、失败收束
- 加载器把 YAML → 内存结构 → 一次性应用到 store + memory（剧本初始化）
- 触发器引擎：每回合结束扫描所有未触发条件，命中则执行动作
- 偏离监测：软引导 / 硬收束两档
- 时间系统：上午 / 下午 / 夜晚 三段；触发器条件一等支持
- 内置《雾港疑案》骨架（自有复述，规避版权）

## Non-Goals

- 不支持用户上传自定义剧本（首发只支持内置；Non-Goals 一致）
- 不支持脚本化分支（YAML 是声明式，不是图灵完备）
- 不实装跨剧本战役（W3 起就否决）

## YAML 顶层结构

```yaml
id: fog_harbor
title: 雾港疑案（骨架）
version: 0.1.0
locations:
  - id: harbor
    name: 雾港码头
    description: 灰雾笼罩的木栈桥...
    connections: [pub, lighthouse]
npcs:
  - id: vance
    name: 范斯医生
    personality: 严肃寡言，眼神回避
    knowledge: { murder: "yes" }
    relation_to_player: 0
    location: pub
clues:
  - id: blood_letter
    description: 沾血的便条，墨迹晕开
items:
  - id: lantern
    name: 黄铜油灯
    description: 老旧但仍可点亮
    owner_type: location
    owner_id: harbor
start:
  location: harbor
  time_of_day: morning
key_clues: [blood_letter, tide_chart, ledger]
triggers:
  - id: lighthouse_storm
    when:
      all:
        - clue_found: blood_letter
        - location_visited: lighthouse
    then:
      - add_event:
          type: scenario
          description: 暴风雨忽至，灯塔下方传来诡异的低鸣
endings:
  - id: solved
    kind: success
    when:
      all:
        - clue_found: blood_letter
        - clue_found: tide_chart
        - clue_found: ledger
        - location_visited: lighthouse
    description: 凶手身份揭晓，调查员带着证据离开雾港
  - id: victim_dies
    kind: failure
    when:
      all:
        - npc_dead: lila
    description: 救援来得太晚——失踪者已无声息
```

## Go 类型（`internal/scenario/types.go`）

`Scenario`、`SLocation`、`SNPC`、`SClue`、`SItem`、`Trigger`、`Action`、`Condition`、`Ending` 一一对应 YAML。

`Condition` 是 union：

```go
type Condition struct {
    All           []Condition `yaml:"all,omitempty"`
    Any           []Condition `yaml:"any,omitempty"`
    Not           *Condition  `yaml:"not,omitempty"`

    LocationVisited string    `yaml:"location_visited,omitempty"`
    ClueFound       string    `yaml:"clue_found,omitempty"`
    NPCDead         string    `yaml:"npc_dead,omitempty"`
    NPCRelationLT   *RelChk   `yaml:"npc_relation_lt,omitempty"` // {npc, value}
    NPCRelationGT   *RelChk   `yaml:"npc_relation_gt,omitempty"`
    TimeOfDay       string    `yaml:"time_of_day,omitempty"`     // morning|afternoon|night
    TurnGE          int       `yaml:"turn_ge,omitempty"`
}
```

`Action` 也是 union：

```go
type Action struct {
    AddEvent          *ActionAddEvent          `yaml:"add_event,omitempty"`
    MarkClueFound     *ActionMarkClueFound     `yaml:"mark_clue_found,omitempty"`
    UpdateNPCRelation *ActionUpdateNPCRelation `yaml:"update_npc_relation,omitempty"`
    KillNPC           string                   `yaml:"kill_npc,omitempty"`
    AdvanceTime       string                   `yaml:"advance_time,omitempty"` // morning→afternoon 等
}
```

YAML 仅一种 case（`Condition` / `Action` 的所有字段都是 omitempty），加载器 Validate 时检查"恰好一种"。

## 触发器引擎

```go
type Engine struct { /* scenario + repo + memory */ }

func (e *Engine) Apply(ctx, scenario, saveID) error  // 初始化：写入 locations/npcs/clues/items 到 store + memory
func (e *Engine) Evaluate(ctx, saveID) ([]FiredTrigger, error)  // 每回合后调用
func (e *Engine) CheckEndings(ctx, saveID) (*Ending, error)     // 每回合后调用
```

Fired 状态用 events 表记录：`type=trigger_fired`、`description=trigger_id`，避免新加表。
`Evaluate` 拉一遍已 fired 列表 → 遍历 scenario.Triggers → 跳过已 fired → 求值 condition → 执行 actions → 写一条 trigger_fired 事件。

## Drift Detector

```go
type DriftStatus int
const (
    DriftOK DriftStatus = iota
    DriftSoft   // 连续 2 回合无主线推进 → 建议 GM 通过环境/NPC 暗示
    DriftHard   // 连续 ≥3 回合 OR 显式拒绝主线 ≥2 次 → 触发预设失败收束
)

type Detector struct { /* scenario + repo */ }
func (d *Detector) Tick(ctx, saveID, currentTurn int) (DriftStatus, error)
```

主线推进定义（MVP）：本回合命中下列任一：
- 找到 `key_clues` 中的任一 → 推进
- 进入未访问过的 location → 推进
- 任一 trigger fired → 推进

orchestrator 在回合结束后调 `Tick`，`DriftSoft` → 注入 prompt 提示；`DriftHard` → 强制走失败收束页（与 SLA #8 同路径）。

## 时间系统

`saves.time_of_day TEXT NOT NULL DEFAULT 'morning'`，取值 `morning|afternoon|night`。

新增两个 tool（在 `internal/orchestrator/tools/`）：
- `get_time_of_day` → 返回当前段
- `advance_time { stages: int = 1 }` → 推进 N 段（morning→afternoon→night→morning 循环；overflow 顺延入次日，但 MVP 不区分日数）

scenario `start.time_of_day` 在 Apply 时落到 saves。

## fog_harbor v0.3.1（可玩剧本，含三 variant）

`internal/scenario/data/fog_harbor.yaml`，2026-05 升至 v0.3.1（v0.3.0 → v0.3.1
专业评审后重构：variant 改为角色站位轮换 + Three Clue Rule 冗余线索 + Anna 受害者面孔）：
- 6 地点：码头、酒馆、灯塔、巡警所、教堂、露西房间、礁洞（subloc）
- 9 NPC：范斯、玛丽莎、海莲娜、欧林、罗克、卡尔文神父、**安娜（下个候选祭品）**、露西、深潜者长老
- 16 线索：4 Tier-1 表层 / 7 Tier-2 共谋 / 4 Tier-3 神话 / 1 red herring（Three Clue Rule 冗余分布）
- 12 触发器：含 2 个时间压力（helena_despairs / rourke_warns）+ Anna 时间炸弹（anna_taken）
- 5 结局：solved / pact_broken / flee_with_truth / victim_dies（含 Anna 死）/ dismissed
- 3 variants（角色站位轮换）：vance_executes / calvin_directs / rourke_runs，每局加权随机选一个

详细设定见 [specs/08-fog-harbor-canon.md](08-fog-harbor-canon.md)。

### 新增可选 schema 字段（v0.3.0）

| 字段 | 位置 | 用途 |
|---|---|---|
| `truth` | Scenario 顶层 | GM-only 真相，注入 `gm_system.tmpl` 不渲染给玩家 |
| `secret` | SNPC | 该 NPC 的隐藏动机；GM 全见，对应 NPC 子代理可见，其他 NPC 不见 |
| `knowledge.requires_phrases` | SNPC.Knowledge | 玩家必须命中关键词，NPC 才"松口"（NPC 子代理自判，引擎不做匹配） |
| `tier` / `location` / `source` / `san_loss` | SClue | tier 1-3 主线 / 0 红鲱；location/source 让 GM 知道线索藏哪 |
| `variants` | Scenario 顶层 | 见下 |

### Variant 系统

`Variant` 描述对 base scenario 的 patch（不允许新增 npc/clue/trigger/ending id，避免破坏校验）：
- `truth` / `npc_secrets` / `npc_knowledge_overrides` / `clue_overrides` / `trigger_overrides` / `ending_desc_overrides`
- `engine.SelectVariant(base, *rand.Rand)` 加权随机选一个；`SelectVariantByID` 用于 CLI `--variant` 强制
- `MergeVariant` 是纯函数，返回新 *Scenario（base 不变）
- store `saves.variant_id` 列持久化本局选择，reload 时按该 id 重新 merge 保持一致性

### 跨周目 meta

`runs/meta.json`（gitignored）记录玩家累计 play_count + 已通关 variants/endings + 已揭开真相。
GM prompt 在新一局注入"玩家先验"段，允许 NPC 出现"似曾相识"的暗示但**不可剧透**。

## 测试

- `loader_test.go`：合法 YAML 加载、必填字段缺失报错、重复 ID 报错、YAML 嵌入资源能 round-trip
- `condition_test.go`：每种 condition + all/any/not 组合
- `engine_test.go`：Apply / Evaluate / 重复触发被去重 / Endings
- `drift_test.go`：连续无推进 → soft → hard
- `fog_harbor_test.go`：内置 YAML 加载 + 端到端跑一段假回合

覆盖率门槛 ≥ 85%。

## Open Questions

- ⏳ key_clues 命名是否要替换为更通用的 "milestones"？保留到 W6 抛光
- ⏳ Trigger 是否需要 once/repeatable 区分？MVP 全部 once
- ⏳ 失败收束的 description 是否要同时附带 ending narrative？保留到 W6 真正接 TUI 时定
