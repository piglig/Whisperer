# Security Policy

## 受支持的版本

Whisperer 仍在 0.x，每个 minor 版本发布后只对**最新**版本提供安全修复。强烈建议
使用最新版。

| 版本 | 受支持 |
|---|---|
| 0.3.x | ✅ |
| < 0.3 | ❌ |

## 报告漏洞

**请不要在公开 issue 里讨论安全漏洞。**

通过 GitHub 的 [私密漏洞报告功能（Security Advisory）](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)
报告：

1. 进入仓库的 [Security 标签](https://github.com/piglig/Whisperer/security)
2. 点击 *Report a vulnerability*
3. 填写漏洞详情、复现步骤、影响评估

我们会在 **3 个工作日内**确认收到，**14 天内**给出初步评估，并与你协调修复
计划与披露时间。

## 安全报告应当包含

- 受影响的组件（如 `internal/store`、`internal/orchestrator` 等）
- 受影响的版本（如 `v0.3.1`）
- 复现步骤——越具体越好
- 影响范围（信息泄露 / 越权写入 / 拒绝服务 / 远程代码执行 / 凭据泄露 / etc）
- 你能想到的修复或缓解方案（可选）

## 我们关心的威胁模型

Whisperer 是单用户单进程的本地 TUI 工具，但仍有几个值得关注的攻击面：

1. **API key 泄露**：日志、trace、错误信息、上传的剧本/存档**绝不**应包含明文 key。
   如果你发现 key 出现在任何文件或网络请求 body 中，这是 bug 也是安全问题。
2. **Prompt injection**：剧本 YAML、用户输入、NPC 知识表都会进入 LLM context；
   恶意构造的剧本可能让 GM 越权执行 tool 或泄露其他 NPC 的 secret。
3. **路径遍历**：剧本热加载（roadmap 中）从用户目录读 YAML；任何允许跨目录引用
   或符号链接逃逸都是问题。
4. **SQL 注入 / 反序列化**：当前所有查询都用占位符，但对外部贡献的代码要继续保持
   这一约束。
5. **依赖链漏洞**：Go 依赖通过 Dependabot 监控；遇到 CVE 我们会发布 patch 版本。

## 我们暂时不视作安全问题的范围

- 把 LLM 输出的"幻觉"当事实接受 —— 这是 LLM 通用问题，非本项目范畴
- 用户用自己的 key 发出超出预算的请求 —— 用户自负
- TUI 渲染异常（颜色错乱、布局错位）—— 提常规 issue 即可

## 致谢

我们会在 release notes 与（如允许）`SECURITY.md` 中列出负责任披露漏洞的报告者。
