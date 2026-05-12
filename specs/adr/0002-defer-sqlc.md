# ADR 0002 — W2 推迟引入 sqlc

## 状态

Accepted（2026-05-12）

## 背景

`specs/00-architecture.md` 与初版 plan 选用 `sqlc` 生成 type-safe 查询代码。开发阶段决定推迟。

## 决策

W2 用标准库 `database/sql` + 手写 thin repository。

## 理由

- 开发阶段（W2-W5）schema 仍会随 NPC 子代理、剧本触发器、事件类型演化
- 每次 schema 变更都需要重跑 `sqlc generate` 并重审生成的 ~千行代码，节奏负担高于 type-safety 收益
- 手写 repository 单文件 ~300 行，可读性高，便于探索期演化
- modernc.org/sqlite + database/sql 已能覆盖全部用例，不存在功能缺口

## 何时重新评估

满足任一条件即重新评估：
- schema 在 ≥ 2 个里程碑内未发生破坏性变更
- repository 行数超过 600 行或重复 boilerplate 显著
- 加入团队成员、需要更强的代码即文档来辅助 onboarding

## 后果

- 写法略 boilerplate（每个 Get 都要 `Scan`），用辅助函数减轻
- 测试需要覆盖所有手写路径；91%+ 覆盖率门槛已能兜底
- 引入 sqlc 时只需替换 repository 的内部实现，对上层 API 透明
