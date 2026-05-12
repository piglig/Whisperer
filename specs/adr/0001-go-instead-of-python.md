# ADR 0001 — 选择 Go 而非 Python

## 状态

Accepted（2026-05-12）

## 背景

需求文档 `llm-rpg-cheerful-moonbeam.md` §6 原选 Python 3.11，理由是 LLM 生态成熟、textual TUI 现代。仓库路径在 `/mnt/d/golang/Whisperer`，与作者的语言偏好（Go）一致。

## 决策

全 Go 实现：
- TUI：bubbletea + lipgloss + bubbles
- LLM SDK：anthropic-sdk-go（官方）
- DB：modernc.org/sqlite（纯 Go、免 cgo） + sqlc
- 向量：chromem-go（嵌入式）
- 日志：log/slog
- 配置：knadh/koanf
- 测试：标准 testing + testify + go-vcr

## 取舍

| 维度 | Python | Go |
|---|---|---|
| LLM 生态 | 更厚（LangChain、LlamaIndex 等） | 较薄但官方 SDK 一等支持 tool use |
| 类型安全 | 动态 | 静态强类型 |
| 部署 | 解释器 + 依赖 | 单二进制 |
| TUI | textual（强） | bubbletea（强） |
| 项目定位（简历项目） | 通用 | 突出 Go 工程能力 |

## 后果

- 不复用 Python LLM 生态的高层抽象（LangChain/LlamaIndex），需要自己实现 tool 路由、RAG 召回，正好对应需求文档"难点是系统设计而非 prompt 调优"
- 结构化检定结果对 SLA 校验更友好（强类型）
- 单二进制 + `embed` 让剧本数据 / prompt 模板 / SQL 迁移直接编译进发布物
