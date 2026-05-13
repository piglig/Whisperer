# Whisperer

> An LLM-driven Call of Cthulhu 7e GM. Single-player. Rules are deterministic. NPCs persist. Cosmic horror is patient.

Whisperer 是一个**单人可玩、由 LLM 驱动、规则确定性可信**的克苏鲁 7e 跑团模拟器。LLM 负责叙事、NPC 扮演、氛围营造；Go 代码负责骰子、规则、状态持久化、SLA 校验。

它不是又一个"无限续写互动小说"。玩家说"我杀了龙"，AI 不会配合"你杀了龙"——而是按 CoC 7e 规则掷骰、判定、按真实结果叙述。

## 与现有方案的差异

|  | AI Dungeon / NovelAI | Whisperer |
|---|---|---|
| 规则裁定 | LLM 自由发挥 | 代码确定性裁决（骰子、技能、SAN 全归 Go） |
| NPC 一致性 | 长 context 漂移 | 子代理 + 向量记忆 + 可选 LLM judge |
| 世界状态 | 文本即真相 | SQLite 持久化 + tool-only 写入 |
| 玩家作弊 | 直接生效 | LLM 不可改数值，只能叙述 |

## 架构（10 行版）

```
TUI (bubbletea)
  ↓
Orchestrator     ── RunTurn ── 推进单回合
  ├── GMAgent (Sonnet)         tool-use 循环
  ├── NPCAgent (Haiku)         单段 NPC 台词
  ├── Tool Dispatcher          20 个 tool 桥接 rules + store + memory
  ├── SLA Validator            4 条结构化 + 可选 LLM-judge 2 条
  ├── Scenario Engine          剧本 YAML + 触发器 + drift detector
  └── Memory (chromem-go)      事件 / NPC / 线索 三集合
        ↑
   Rules Engine (rules)        纯函数：骰子、技能、SAN、对抗、战斗、成长
   State Store  (store)        SQLite + 单文件存档
```

完整模块边界与契约见 [`specs/00-architecture.md`](specs/00-architecture.md)。

## 运行

### 依赖

- Go 1.22+（项目用 1.23 开发）
- Anthropic API key（`ANTHROPIC_API_KEY` 环境变量）

```bash
go build ./cmd/whisperer
./whisperer --help
```

### 启动一局

支持两种 LLM 提供商：

```bash
# Anthropic 官方
export ANTHROPIC_API_KEY=sk-ant-...
./whisperer                                    # 默认：whisperer.db + ./mem + fog_harbor

# OpenRouter（Anthropic 兼容端点 + Bearer token）
export OPENROUTER_API_KEY=sk-or-...
./whisperer --provider openrouter
```

provider 探测规则：
- 仅设了 `OPENROUTER_API_KEY` → 自动用 OpenRouter
- 否则 → Anthropic 官方
- 总是可以用 `--provider {anthropic|openrouter}` + `--api-key <k>` 显式覆盖

OpenRouter 模式下，模型名会自动加 `anthropic/` 前缀（如 `anthropic/claude-sonnet-4-5-20250929`）。

不带 `--save` 时自动新建存档 + 一名占位调查员 + 写入剧本初始数据。

### Smoke check（不调 LLM）

```bash
./whisperer --smoke
```

仅验证编译 + 依赖装载，无网络请求。

### 命令行参数

| flag | 默认 | 说明 |
|---|---|---|
| `--db` | `whisperer.db` | SQLite 存档文件 |
| `--memory` | `mem` | chromem-go 持久化目录；空字符串 = in-memory |
| `--scenario` | `fog_harbor` | 内置剧本 id |
| `--save` | _（空）_ | 已有 save id；缺省自动新建 |
| `--provider` | 自动 | `anthropic` 或 `openrouter` |
| `--api-key` | env | 显式覆盖；缺省读对应 provider 的 env |
| `--smoke` | `false` | 仅 smoke check，不启 TUI |

### TUI 命令

| 命令 | 行为 |
|---|---|
| `/sheet` | 调查员属性 |
| `/inventory` (`/inv`) | 背包 |
| `/time` | 当前时段（上午/下午/夜晚） |
| `/talk <NPC> <话>` | 指名对某 NPC 说 |
| `/all <话>` | 对全场说 |
| `/hint` | 求助：GM 仅给环境/NPC 暗示，不剧透 |
| `/bind <名字> <职业>` | 仅在结局后：把新调查员接续到本剧本 |
| `/help` | 帮助 |
| `/quit` (`/exit` / `/q`) | 退出 |

## 重开性（v0.3.0 新）

雾港疑案不是一份静态 YAML——它是 **base + 3 variants + 跨周目 meta** 的组合。每局开始
随机选一个 variant 决定真凶/共谋/线索分布；玩家通关后下一局，NPC 会出现"似曾相识"
的暗示（不剧透）。三层重开性叠加：

| 维度 | 数量 | 贡献 |
|---|---|---|
| variant（角色站位轮换） | 3 | vance_executes / calvin_directs / rourke_runs |
| 结局分支 | 5 | solved / pact_broken / flee_with_truth / victim_dies / dismissed |
| 关键词解锁的 NPC 隐藏知识 | 8 NPC × ~3 entries | 玩家用语言探索 |
| 跨周目 meta 暗示 | runs/meta.json | NPC 第 2/3 局对玩家"似曾相识" |

保守估 **10–15 局新鲜感**。

CLI flags：
- `--variant <id>` 强制指定 variant（默认按权重随机）
- `--seed <n>` 指定随机种子，便于回放/确定性测试
- `--meta <path>` 跨周目 meta 文件路径（默认 `runs/meta.json`，`-` 关闭）

完整设定：[specs/08-fog-harbor-canon.md](specs/08-fog-harbor-canon.md)。

## 已实现（与需求文档 §3 Goals 对齐）

- [x] **G1** 雾港疑案 v0.3.1 完整剧本（自有原创；6 地点 / 9 NPC / 16 线索 / 12 触发器 / 5 结局 / 3 variants；Three Clue Rule + Anna 受害者面孔 + 角色站位 variant）
- [x] **G2** 骰子 / 技能检定 / SAN / 战斗全部由代码裁决；LLM 通过 tool 调用，不允许私自宣判
- [x] **G3** NPC 子代理 + 向量记忆，跨回合保持人格（结构化 + 可选 LLM judge）
- [x] **G4** 世界状态持久化（SQLite + tool-only 写入）
- [x] **G5** 单文件 SQLite 即一份存档；启动时 `--save <id>` 续读
- [x] **G6** 调查员死亡/不定性疯狂触发结局页；`/bind` 命令绑定新调查员（剧本进度保留）
- [x] **G7** 剧本结束 settle_growth；rules.SettleGrowth 已实装

## 未实现 / Non-Goals

| 项 | 状态 | 说明 |
|---|---|---|
| 多人联机 | ❌ | Non-Goals |
| 多角色编队 | ❌ | Non-Goals |
| 战斗深度（护甲、闪避、格挡） | ❌ | Non-Goals；MVP 仅含先攻、命中、伤害、impale |
| AI 生成图像 / 语音 | ❌ | Non-Goals |
| 用户上传自定义剧本 | ❌ | 首发只支持内置剧本 |
| 移动端 / GUI | ❌ | Non-Goals |
| 其他规则系统（D&D 等） | ❌ | Non-Goals |
| 跨剧本战役 | ❌ | Non-Goals |
| `/redo` 完整实现 | ⏳ | W7 推迟到独立 milestone（涉及历史 rewind） |
| 全 24 职业模板分步建卡（F1.4） | ⏳ | 当前是快速生成 + 占位调查员 |
| Autosave 回滚 | ⏳ | 已写 checkpoint 事件；文件克隆式回滚未做 |
| LLM-as-judge 默认开启 | ⏳ | 接口 + Haiku 实现已就位；默认关闭，待评估 |

## 项目布局

```
Whisperer/
├── cmd/whisperer/              CLI 入口
├── internal/
│   ├── rules/                  纯函数规则引擎（W1）
│   ├── store/                  SQLite + 迁移（W2）
│   ├── agent/                  Anthropic SDK 封装 + GM/NPC agent（W3 W4）
│   ├── memory/                 chromem-go 三集合（W4）
│   ├── scenario/               YAML 剧本 + 触发器 + drift（W5）
│   ├── orchestrator/           回合主循环 + tools + SLA（W6 W7）
│   │   ├── tools/              20 个 tool 桥接 rules / store / memory
│   │   └── sla/                4 条结构化 SLA + 可选 LLM judge
│   └── tui/                    bubbletea 前端（W6）
├── specs/                      逐周 spec 文档 + ADR
└── runs/                       运行时 JSONL 日志（gitignore）
```

## 测试 / 质量

```bash
make test    # 全测试
make cover   # 覆盖率（门槛 85%）
make build   # 单二进制
make lint    # vet + golangci-lint（如装）
```

最近一次基准：

| 包 | 覆盖率 |
|---|---|
| rules | 91.2% |
| store | 88.1% |
| agent | 89.1% |
| memory | 89.7% |
| scenario | 89.8% |
| orchestrator | 90.8% |
| orchestrator/tools | 93.1% |
| orchestrator/sla | 93.7% |
| tui | 93.0% |
| **整体** | **~90%** |

## SLA 校验

LLM 受 8 条 GM 行为契约约束（见 [`specs/06-orchestrator-and-tui.md`](specs/06-orchestrator-and-tui.md)）。每回合输出过校验器；失败 → 注入 `<sla_violation>` 让 GM 重写 narrative，不滚回状态变更。

| # | SLA 项 | 实现 |
|---|---|---|
| 1 | 检定即工具 | 结构化（关键词 + tool 检测） |
| 2 | 状态即工具 | 结构化（同上，多对应 tool） |
| 3 | NPC 一致性 | LLM-judge（可选） |
| 4 | 物品守恒 | 结构化（destroyed_items 反向匹配） |
| 5 | 数值不采信 | （MVP 待补） |
| 6 | 失败即失败 | 结构化（关键词反向匹配） |
| 7 | 角色知识闭环 | LLM-judge（可选） |
| 8 | 失败收束阻断 | 结构化（hp/san/active 检查） |

## 设计决策（精选 ADR）

- [ADR 0001](specs/adr/0001-go-instead-of-python.md) — 用 Go 而非需求文档原写的 Python
- [ADR 0002](specs/adr/0002-defer-sqlc.md) — W2 推迟 sqlc，schema 稳定后再评估

## License

私人项目，未公开发布。
