// Package secrets 提供把"看起来像 API key / token"的子串从任意文本里抹掉的工具
// 函数。
//
// 用途：在写入 TraceEntry.UserInput / 事件描述 / 日志 attr 值之前过一遍，避免
// 玩家在 TUI 里粘错东西、或 LLM 把 system prompt 中提到的 key 反刍出来时把秘密
// 落到磁盘 / 仓库 / 公开 trace。
//
// **不是密码学保证**——只用启发式 regex。已知能匹配的形态：
//   - sk-ant-api03-XXXX...        Anthropic 官方
//   - sk-or-v1-XXXX...            OpenRouter
//   - sk-XXXX...                  OpenAI 类
//   - Bearer <长 base64-ish>      Authorization 头形态
//   - eyJ...XXX...                JWT (3 段 base64url，至少 20 字符 segments)
//
// 不能匹配的形态（设计取舍——避免误伤普通文本）：
//   - 短随机 hex / base64 子串
//   - 用户自定义命名的 key（"我的-api-key-2026"）
package secrets

import (
	"regexp"
)

// 匹配主流 LLM provider 的 API key 形态。每条独立 regex 以便后续扩展。
var keyPatterns = []*regexp.Regexp{
	// sk-ant-api03-xxxx... / sk-ant-... / 任何 sk-ant- 前缀
	regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]{16,}`),
	// sk-or-v1-... / OpenRouter
	regexp.MustCompile(`sk-or-[A-Za-z0-9_-]{16,}`),
	// OpenAI 类（sk-proj- / sk-svcacct- / 老式 sk-xxxx）
	regexp.MustCompile(`sk-(?:proj|svcacct)?-?[A-Za-z0-9_-]{20,}`),
	// Bearer 头 + 长 token
	regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._\-]{20,}`),
	// JWT 三段 base64url
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`),
}

// Redact 扫描 s 中所有看起来像 API key 的子串并替换为 "***REDACTED***"。
// 没有命中时原样返回。
func Redact(s string) string {
	if s == "" {
		return s
	}
	out := s
	for _, re := range keyPatterns {
		out = re.ReplaceAllString(out, "***REDACTED***")
	}
	return out
}
