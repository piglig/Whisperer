# Whisperer

[English](README.md) | [简体中文](README.zh-CN.md)

Whisperer 是一个本地运行的单人调查型 TRPG 引擎。LLM 扮演主持人
（GM），Go 代码负责骰子、技能、理智、战斗、状态流转、剧本触发、内容验
收和回放调试。

当前内置剧本是 **Fog Harbor**（`fog_harbor`），这是一个原创的克苏鲁风格
谜案，基于确定性的 d100 规则运行。

> Whisperer 是非官方同人项目。*Call of Cthulhu* 归 Chaosium Inc. 所有。
> 本仓库不隶属于 Chaosium，也未获得其背书；不会分发 Chaosium 已出版剧本、
> 美术或角色数据。

## 特性

- **确定性规则**：骰子、HP、SAN、技能、线索、物品移动和结局由 Go 工具
  解析，不由模型临场编造。
- **剧本创作闭环**：`scenario lint`、确定性 playtest 和内容验收门槛帮助
  作者保持剧本可发布。
- **可观测性优先**：每回合都可以写入 JSONL trace，支持文本回放、HTML 交
  互式回放，以及导出 playtest script。
- **LLM 护栏**：结构化 SLA 检查和可选的 LLM-as-judge 检查可以捕获缺失
  tool call、知识泄漏、失败检定被改写、强制结局等问题。
- **可重玩性**：Fog Harbor 包含变体、多结局、分阶段线索和跨周目的 meta
  memory。

## 项目状态

Whisperer 正处于活跃开发阶段。核心架构仍在快速收敛，因此当前不承诺兼容
性。

主要开发链路：

| 链路 | 命令入口 | 目标 |
|---|---|---|
| Runtime | `whisperer` | 面向玩家的 TUI 游戏循环 |
| Authoring | `whisperer scenario lint|playtest|verify` | 剧本校验和内容验收 |
| Observability | `whisperer e2e`, `whisperer replay` | LLM 验证、trace 复盘、报告 |

## 环境要求

- Go 1.25.8 或更新版本
- 真实游玩需要至少一个受支持的 LLM provider key
- 语义记忆可选 OpenAI-compatible embedding provider

支持的 LLM provider：

- Anthropic
- OpenRouter
- OpenAI
- Grok / xAI
- Gemini

## 安装

从源码构建：

```bash
git clone https://github.com/piglig/Whisperer.git
cd Whisperer
go build -o whisperer ./cmd/whisperer
./whisperer --help
```

项目也提供首次运行配置向导：

```bash
./whisperer init
```

## 配置

Whisperer 会从以下位置读取配置：

- `$XDG_CONFIG_HOME/whisperer/config.toml`
- `$HOME/.config/whisperer/config.toml`
- Windows 上的 `%APPDATA%/whisperer/config.toml`
- `--config /path/to/config.toml`

优先级：

```text
CLI flags > environment variables > config file > built-in defaults
```

API key 应通过环境变量提供，不应写入配置文件：

```bash
export ANTHROPIC_API_KEY=sk-ant-...
export OPENROUTER_API_KEY=sk-or-...
export OPENAI_API_KEY=sk-...
export XAI_API_KEY=xai-...
export GEMINI_API_KEY=...
```

完整配置示例见 [docs/example-config.toml](docs/example-config.toml)。

## 快速开始

启动默认 TUI：

```bash
./whisperer
```

显式选择 provider：

```bash
./whisperer --provider openai
./whisperer --provider openrouter
./whisperer --provider gemini
```

创建可复现的调试运行：

```bash
./whisperer --scenario fog_harbor --variant calvin_directs --seed 42
```

使用不同调查员模板：

```bash
./whisperer --investigator-template private_eye
./whisperer --investigator-template doctor
```

继续存档：

```bash
./whisperer --save <save-id>
```

## Runtime 操作

TUI 以案件板和可行动项为主交互。玩家可以直接选择编号行动，也可以继续输入
自由文本。

常用 slash commands：

| 命令 | 说明 |
|---|---|
| `/talk <NPC>` | 对指定 NPC 说话 |
| `/all <text>` | 对当前场景内所有人说话 |
| `/hint` | 请求非剧透提示 |
| `/bind <name> <occupation>` | 角色死亡后绑定替代调查员 |
| `/help` | 显示游戏内帮助 |

被拦截的行动会通过结构化 decision 对象返回，其中包含 reason code、玩家可
读解释、debug reason、所需线索和建议恢复行动。

## Authoring 命令

检查剧本结构：

```bash
./whisperer scenario lint --scenario fog_harbor
./whisperer scenario lint --all
./whisperer scenario lint --scenario fog_harbor --json
```

运行确定性 playtest：

```bash
./whisperer scenario playtest --scenario fog_harbor --path mainline
./whisperer scenario playtest --scenario fog_harbor --all
./whisperer scenario playtest --scenario fog_harbor --all --json
```

运行完整内容验收门槛：

```bash
./whisperer scenario verify --scenario fog_harbor
./whisperer scenario verify --scenario fog_harbor --json
```

Authoring adapter 位于 `internal/authoring.ScenarioAdapter` 后面。新增剧本
不应需要新增 CLI 命令。

## Observability 命令

不启动 TUI，直接运行真实 LLM e2e 验证：

```bash
./whisperer e2e --provider openai --input "我环顾码头四周"
./whisperer e2e \
  --inputs-file cmd/whisperer/e2e_scripts/fog_harbor_mainline.txt \
  --turns 25 \
  --enable-judge
```

回放 trace：

```bash
./whisperer replay runs
./whisperer replay --turn 7 runs/<save-id>/<session>.jsonl
./whisperer replay --html tmp/replay.html runs/<save-id>/<session>.jsonl
./whisperer replay --export-script tmp/playtest.txt runs/<save-id>/<session>.jsonl
./whisperer replay --turn 7 --mark bad,misjudge --note "NPC contradicted a clue" runs/<save-id>/<session>.jsonl
```

Replay 输出包含玩家输入、GM 输出、tool calls、SLA、judge、状态 diff、结
局、行动拦截解释和人工标记。

## 语义记忆和 Judge

启用 OpenAI-compatible embeddings：

```bash
export OPENAI_API_KEY=sk-...
./whisperer --embedder openai --embedder-model text-embedding-3-small
```

使用其他兼容 endpoint：

```bash
export VOYAGE_API_KEY=...
./whisperer \
  --embedder openai_compat \
  --embedder-base-url https://api.voyageai.com/v1 \
  --embedder-model voyage-3-large \
  --embedder-api-key-env VOYAGE_API_KEY
```

常用开发模式：

```bash
./whisperer --embedder fake
./whisperer --embedder off
./whisperer --enable-judge
```

## 仓库结构

```text
Whisperer/
├── cmd/whisperer/          Runtime、Authoring 和 Observability CLI
├── docs/                   面向用户的示例
├── internal/
│   ├── agent/              LLM providers、GM/NPC agents、cassettes
│   ├── authoring/          剧本 adapter registry、playtests、gates
│   ├── config/             TOML/env/flag 配置
│   ├── director/           剧本节奏和建议
│   ├── fogharbor/          Fog Harbor adapter 和路径脚本
│   ├── memory/             语义记忆和 embedders
│   ├── orchestrator/       回合循环、action decisions、SLA、tools
│   ├── replay/             Trace 文档模型和 HTML viewer
│   ├── rules/              纯 d100 规则
│   ├── scenario/           YAML loader、variants、triggers、reports
│   ├── store/              SQLite 持久化
│   └── tui/                Bubble Tea 终端 UI
├── specs/                  架构、子系统参考和 ADR
└── runs/                   本地 traces 和 meta state，已 gitignore
```

## 开发

```bash
make build
make test
make cover
make lint
```

等价的直接命令：

```bash
go build ./...
go test ./...
go test -race ./...
go vet ./...
```

## 文档

- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)
- [Changelog](CHANGELOG.md)
- [Architecture overview](specs/00-architecture.md)
- [Scenario authoring](specs/05-scenario-and-triggers.md)
- [Fog Harbor canon](specs/08-fog-harbor-canon.md)
- [Original product brief](llm-rpg-cheerful-moonbeam.md)

## 许可证

Apache-2.0。详见 [LICENSE](LICENSE)。
