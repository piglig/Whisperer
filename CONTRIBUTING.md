# Contributing to Whisperer

感谢你愿意为 Whisperer 出力。这个项目欢迎任何形式的贡献——bug 报告、文档修正、
新剧本投稿、规则模块扩展、UI/UX 建议都好。

## 行为准则

参与前请阅读 [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md)。简单说：对事不对人，
不歧视，不骚扰。

## 开发环境

依赖：
- Go 1.25+
- 一个 LLM provider 的 API key（仅在跑 `cmd/e2esmoke` 真机端到端测试时需要）
  - `ANTHROPIC_API_KEY` 走官方
  - `OPENROUTER_API_KEY` 走 OpenRouter

构建与测试：

```bash
git clone https://github.com/piglig/Whisperer.git
cd Whisperer
make build      # 编译
make test       # go test -race ./...
make cover      # 覆盖率（门槛 85%）
make lint       # go vet + golangci-lint（如装）
```

不调 LLM 的冷启动检查：

```bash
./whisperer --smoke
```

## 提交流程

1. **开 issue 先于写代码**——除非是 typo / 一行 bugfix。让维护者与你对齐方向能省下重写。
2. **从 `main` fork** 一个 feature 分支（命名建议：`fix/<short>` / `feat/<short>` /
   `docs/<short>`）。
3. **保持 commit 干净**：每个 commit 自包含、能编译、能跑测试。如果一次工作做了多个
   独立改动（比如修了 bug + 顺手加了文档），分多个 commit。
4. **commit 信息**：第一行 ≤ 72 字，祈使句（"add ledger validation" 而不是 "added"）；
   正文解释 *为什么*，不是 *是什么*（diff 已经写了 *是什么*）。
5. **PR 描述**：
   - 一段说清解决的问题
   - 关联的 issue 号
   - 测试方式（手动 / 单测 / e2esmoke）
   - 任何破坏性变更或迁移要点

## 代码风格

- `go fmt` 通过；`go vet` 不报；`golangci-lint run` 干净
- 公有 API 必须有 godoc 注释（中英都行，与现有风格一致）
- 测试覆盖率门槛 **85%**——`make cover` 要绿
- 错误用 `fmt.Errorf("context: %w", err)` 包装；不要丢上下文
- 不引入新的第三方依赖时优先 stdlib；引入前在 PR 里说明权衡
- 避免 `panic`——除非是真正不可恢复的初始化错误（`init()` / 程序启动时的配置错误等）

## 写新剧本

YAML schema 与设计指南详见
[`specs/05-scenario-and-triggers.md`](specs/05-scenario-and-triggers.md) 与
[`specs/08-fog-harbor-canon.md`](specs/08-fog-harbor-canon.md)。

剧本设计的几条硬规则：
1. **Three Clue Rule**：每个关键结论 ≥ 3 条独立线索。否则玩家漏 1 条就卡死。
2. **变量化重开性**：用 `variants` 实现"角色站位轮换"而不是"换凶手"——避免世界
   一致性崩塌（多 variant 必须共享同一份历史）。
3. **时间压力**：用 `turn_ge` + `not trigger_fired` 的触发器制造取舍点；不要让玩家
   能无限磨蹭。
4. **NPC 用 `requires_phrases` 解锁知识**：比单纯关系阈值更耐玩；玩家需要"用语言探索"。

提交剧本到 `internal/scenario/data/`，附 `scenarios/<name>_canon.md`（GM-only 真相
手册）+ 至少一份 e2esmoke 主线脚本。

## 报 bug

模板在 `.github/ISSUE_TEMPLATE/bug.yml`。请尽量包含：
- whisperer 版本（`whisperer --version`）/ Go 版本 / OS
- 复现步骤
- 期望 vs 实际
- 必要时附 trace（`runs/<save_id>/<ts>.jsonl`，把 API key 抹掉）

## 提建议 / 提需求

模板在 `.github/ISSUE_TEMPLATE/feature.yml`。先描述场景再描述方案，避免 XY 问题。

## 安全问题

**不要**在公开 issue 里讨论安全漏洞。流程见 [`SECURITY.md`](SECURITY.md)。

## License

提交即同意贡献内容以 [Apache-2.0](LICENSE) 授权。
