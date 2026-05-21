# Whisperer

> 让 LLM 当跑团 GM，让代码当裁判。
> *An LLM-driven, single-player Cthulhu-flavored TRPG simulator with deterministic rules.*

[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Tests](https://img.shields.io/badge/tests-passing-brightgreen)](#测试--质量)
[![Coverage](https://img.shields.io/badge/coverage-%3E85%25-brightgreen)](#测试--质量)

```
┌─ 雾港疑案 · 第 7 回合 · night ─────────────────────────────┐
│ 你蹲在码头边，潮水正在退去。桩柱根部夹着一块湿透的布料——     │
│ 海莲娜形容露西失踪夜穿的就是同样的花纹。你把它装进证物袋，   │
│ 海风像在你耳边低语。                                       │
│                                                           │
│ [Spot Hidden 60 → 47 success]                             │
│ [SAN 60 → 59 (-1)]                                        │
│                                                           │
│ > 我把布料拿给海莲娜看_                                     │
└───────────────────────────────────────────────────────────┘
```
*↑ 录屏占位 — `docs/demo.cast` 上线后改 asciinema*

---

## 这是什么

**Whisperer 是一个跑团 GM 程序**：你在终端里输入"我向酒馆老板娘打听"，
它扮演整个克苏鲁式小镇——叙事、NPC 对话、氛围营造由 LLM 驱动；
**骰子、技能检定、SAN 损失、HP 流转一律由 Go 代码裁决**，LLM 不能私自改数。

它不是另一个"无限续写互动小说"。玩家说"我杀了龙"，AI 不会配合"你杀了龙"——
而是按 d100 规则掷骰、判定、按真实结果叙述。

**3 句话讲清差别**：
- ⚖️ **规则确定性**：骰子 / 技能 / SAN / 战斗全在 Go；LLM 只能调 tool，不能改数值
- 🧠 **NPC 持久化**：子代理 + 向量记忆 + 关键词解锁知识，跨回合不漂移
- 🎲 **重开性 ≥ 10 局**：3 个 variant × 5 结局 × 跨周目"似曾相识" meta

> **Fan disclaimer**: Whisperer is an unofficial fan project. *Call of Cthulhu*® is © Chaosium Inc.
> This project is not affiliated with or endorsed by Chaosium. No published Chaosium content
> (printed scenarios, illustrations, NPC stat blocks from books) is redistributed; only
> public-domain rule mechanics (d100, generic skill names) and original scenarios are used.

---

## 安装

> 多平台二进制 / Homebrew / Scoop 在 Phase 3 (release pipeline) 上线。当前从源码构建：

```bash
git clone https://github.com/piglig/Whisperer.git && cd Whisperer
go build -o whisperer ./cmd/whisperer
./whisperer --help
```

### LLM provider

支持 Anthropic 官方与 OpenRouter（Anthropic 兼容端点）：

```bash
# 官方
export ANTHROPIC_API_KEY=sk-ant-...
./whisperer

# OpenRouter
export OPENROUTER_API_KEY=sk-or-...
./whisperer --provider openrouter
```

`--provider` 不指定时自动探测：仅 OpenRouter key 走 OpenRouter，否则官方。
`--api-key <k>` 显式覆盖。

---

## Quickstart

```bash
# 默认：自动建档 + 占位调查员 + fog_harbor 剧本 + 加权随机选 variant
./whisperer

# 强制选某 variant + 固定种子（便于回放调试）
./whisperer --variant=calvin_directs --seed=42

# 续读已有存档
./whisperer --save <save-id>

# 不调 LLM 的冷启动检查
./whisperer --smoke
```

进入 TUI 后输入你想做的事即可（自然语言）。常用命令：

| 命令 | 作用 |
|---|---|
| `/talk <NPC>` | 显式指定 NPC 对话 |
| `/all <话>` | 对全场说 |
| `/hint` | 卡住时让 GM 给环境/NPC 暗示（不剧透） |
| `/bind <名> <职业>` | 调查员死亡后接续新角色到本剧本 |
| `/help` | 命令帮助 |

---

## 当前剧本：《雾港疑案》

一桩边远港口的失踪案。表面是少女露西的母亲求助，背后是三十年的契约。

- **6 个调查地点**（码头 / 酒馆 / 灯塔 / 巡警所 / 教堂 / 礁洞）
- **9 个 NPC**，每位都有秘密 + 关键词解锁的隐藏知识
- **16 条线索**，按 Tier 1（表层）/ Tier 2（共谋）/ Tier 3（神话）三层递进 —— 遵循
  [Three Clue Rule](https://thealexandrian.net/wordpress/1118/roleplaying-games/three-clue-rule)：
  每个关键结论 ≥ 3 条独立线索路径
- **3 个 variant**（`vance_executes` / `calvin_directs` / `rourke_runs`）每局加权随机
  选一个，**轮换"当代谁是执行者"而不破坏世界一致性**
- **5 类结局**：`pact_broken`（完美）/ `solved`（标准成功）/ `flee_with_truth`（带证据撤离）/
  `victim_dies`（救人失败）/ `dismissed`（被驱逐）；调查员死亡走系统级尸检页
- **跨周目 meta**（`runs/meta.json`）：玩家通关后下局 NPC 会出现"似曾相识"反应

完整剧本设定与真相手册：[`specs/08-fog-harbor-canon.md`](specs/08-fog-harbor-canon.md)。

---

## 它怎么工作的

```
TUI (bubbletea)
  ↓
Orchestrator      RunTurn 推进单回合
  ├── GMAgent (Sonnet)        tool-use 循环
  ├── NPCAgent (Haiku)        单段 NPC 台词，独立 system prompt
  ├── Tool Dispatcher         20+ 个 tool 桥接 rules / store / memory
  ├── SLA Validator           4 条结构化 + 可选 LLM judge × 2
  ├── Scenario Engine         YAML 剧本 + 触发器 + 三幕节奏 + drift 检测
  └── Memory (chromem-go)     事件 / NPC / 线索 三集合向量库
       ↑
  Rules Engine (rules)        纯函数：d100 / 技能 / SAN / 对抗 / 战斗 / 成长
  State Store  (store)        SQLite + 单文件存档（含 variant / meta）
```

设计准则与契约边界详见 [`specs/00-architecture.md`](specs/00-architecture.md)。

---

## 项目布局

```
Whisperer/
├── cmd/
│   ├── whisperer/                CLI / TUI 入口
│   └── e2esmoke/                 真机 LLM 端到端跑步机
├── internal/
│   ├── rules/                    纯函数规则引擎
│   ├── store/                    SQLite + repository
│   ├── agent/                    Anthropic SDK + GM/NPC agent + prompts
│   ├── memory/                   chromem-go 三集合
│   ├── scenario/                 YAML 剧本 + 触发器 + variant + meta
│   ├── orchestrator/             回合主循环 + tools + SLA + judge
│   └── tui/                      bubbletea 前端
├── specs/                        架构 spec + ADR + 剧本 canon
└── runs/                         运行时 trace / meta（gitignored）
```

---

## 测试 / 质量

```bash
make test    # go test -race ./...
make cover   # 覆盖率，门槛 85%
make build
make lint    # vet + golangci-lint
```

最近一次覆盖率：

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

---

## 路线图

- ✅ v0.3.x — 引擎 + Fog Harbor v0.3.1（variant 角色站位 + Three Clue Rule + Anna）
- 🔜 v0.4.0 — **工程成熟度**：CI / migrations (goose) / 结构化日志 (slog) / 真 embedder /
  cost 计费 / OTEL spans / LLM 录放（go-vcr）
- 🔜 v0.5.0 — **用户体验**：onboarding wizard / 友好错误 / i18n (CN+EN) / shell completion /
  scenario 热加载
- 🔜 v0.6.0 — **分发**：goreleaser 多平台 + Homebrew tap + Scoop bucket + 一行安装脚本
- 🔜 v0.7.0+ — 内容与社区：第二/第三剧本 / scenario linter / 文档站 / asciinema demo

详细路线图与决策记录：[`specs/00-architecture.md`](specs/00-architecture.md) +
[`CHANGELOG.md`](CHANGELOG.md)。

---

## 贡献

欢迎 issue 与 PR。开发流程、commit 风格、剧本编写指南详见
[`CONTRIBUTING.md`](CONTRIBUTING.md)。

参与社区前请阅读 [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md)；
发现安全问题请通过 [`SECURITY.md`](SECURITY.md) 描述的私下渠道报告。

---

## License

[Apache License 2.0](LICENSE) © Whisperer contributors.
