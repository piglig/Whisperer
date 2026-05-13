<!--
感谢 PR！请填写下面的简表。删除不相关的章节即可。
-->

## 这个 PR 解决了什么

<!-- 一段说明问题 / 动机；关联 issue 用 `Closes #123` / `Refs #45`。 -->

## 改动概要

<!-- bullet 列出主要改动；强调"为什么这样改"，不是 diff 已经写明的"是什么"。 -->

-
-

## 测试方式

<!-- 至少一项。 -->

- [ ] `make test` 全绿
- [ ] `make cover` ≥ 85%
- [ ] `make lint` 干净
- [ ] 真机 e2esmoke 跑了 mainline（如改动涉及 LLM 路径）
- [ ] 手动验证：<描述步骤>

## 破坏性变更

<!-- 是否破坏了已有 save、剧本格式、配置语义、CLI 标志？描述迁移路径。 -->

- [ ] 无破坏性变更
- [ ] 有破坏性变更（描述 ↓）：

## Checklist

- [ ] 我读过 [CONTRIBUTING.md](../CONTRIBUTING.md)
- [ ] commit 信息祈使句 + 解释 *为什么*
- [ ] 公有 API 有 godoc 注释
- [ ] CHANGELOG 在 `[Unreleased]` 段加了一行
- [ ] 不引入新依赖；如引入已在 PR 描述里说明权衡
