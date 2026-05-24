// Package clierror 把内部 error 翻译成面向终端用户的友好提示。
//
// 设计取舍：
//   - 不返回新的错误类型，避免污染内部签名；只在 CLI 入口调用 Format(err) 渲染输出。
//   - 已知错误模式按 errors.As / 字符串子串两路检测（SDK 错误类型 + 网络错误
//     文本特征）。命中则查 i18n 表得到提示；未命中走 error.unknown 模板。
//   - 不做任何 stack trace / 调试 dump——那是 slog debug 级别该出的。CLI 错误
//     输出只有"发生了什么"+"建议下一步"。
package clierror

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/zhuzhenwu/whisperer/internal/i18n"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

// Format 把 err 渲染为面向终端用户的多行字符串：第一行 "Op 失败"，第二行起的
// 缩进是定位提示。op 是当前操作的简短名（例如 "open store" / "build orchestrator"）。
//
// tr 为 nil 时退化为原始错误字符串，避免 nil-deref。
func Format(tr *i18n.Translator, op string, err error) string {
	if err == nil {
		return ""
	}
	if tr == nil {
		return fmt.Sprintf("%s: %s", op, err.Error())
	}

	hint, kind := classify(err)
	if kind == "" {
		return tr.T("error.unknown", map[string]any{
			"Op":     op,
			"Reason": err.Error(),
		})
	}
	return hint(tr, op, err)
}

// classify 走一组探针，返回 (hint 渲染函数, 模式名)。模式名空表示未识别。
func classify(err error) (func(*i18n.Translator, string, error) string, string) {
	// === Anthropic SDK / OpenRouter HTTP 类错误 ===
	// 注意：apiErr.Error() 在 Request 字段未填时会 panic（SDK 内部 fmt 触碰
	// nil request）。所以我们只用 StatusCode 与 RawJSON，绝不调 .Error()。
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 401, 403:
			return renderAPIKeyInvalid, "api_key_invalid"
		case 429:
			return renderQuotaExceeded, "quota_exceeded"
		case 404:
			if strings.Contains(strings.ToLower(apiErr.RawJSON()), "model") {
				return renderModelNotAllowed, "model_not_allowed"
			}
		}
	}

	// === 网络层 ===
	if isNetworkError(err) {
		return renderNetwork, "network"
	}

	// === Whisperer 自有 sentinel ===
	if errors.Is(err, store.ErrNotFound) {
		// store.ErrNotFound 在 save / scenario / npc 等多场景出现；按错误链上的
		// 字符串嗅探具体上下文。
		if strings.Contains(strings.ToLower(err.Error()), "save") {
			return renderSaveNotFound, "save_not_found"
		}
	}
	return nil, ""
}

// ============================================================================
// renderers
// ============================================================================

func renderAPIKeyInvalid(tr *i18n.Translator, op string, err error) string {
	provider := guessProvider(err)
	return wrap(op, tr.T("error.api_key_invalid", map[string]any{"Provider": provider}))
}

func renderQuotaExceeded(tr *i18n.Translator, op string, err error) string {
	provider := guessProvider(err)
	return wrap(op, tr.T("error.quota_exceeded", map[string]any{"Provider": provider}))
}

func renderModelNotAllowed(tr *i18n.Translator, op string, err error) string {
	model := "?"
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		model = extractQuoted(apiErr.RawJSON(), "model")
	}
	return wrap(op, tr.T("error.model_not_allowed", map[string]any{"Model": model}))
}

func renderNetwork(tr *i18n.Translator, op string, err error) string {
	return wrap(op, tr.T("error.network", map[string]any{"Reason": err.Error()}))
}

func renderSaveNotFound(tr *i18n.Translator, op string, err error) string {
	saveID := extractQuoted(err.Error(), "save")
	return wrap(op, tr.T("error.save_not_found", map[string]any{"SaveID": saveID}))
}

// ============================================================================
// helpers
// ============================================================================

// wrap 把 op 与 hint 拼成两行文本：op 行在上，hint 行在下，无空行——便于 grep。
func wrap(op, hint string) string {
	return op + ": " + hint
}

// guessProvider 从 SDK error 的 RawJSON 嗅探 provider 名。命不中返回 "anthropic"。
//
// 直接对 err.Error() 做嗅探不安全——SDK 的 *anthropic.Error 在 Request 未填时
// .Error() 会 panic；我们只取 RawJSON 与 wrapper 链上的非 SDK 错误字符串。
func guessProvider(err error) string {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		if strings.Contains(strings.ToLower(apiErr.RawJSON()), "openrouter") {
			return "openrouter"
		}
		return "anthropic"
	}
	// 普通错误：可以安全调 .Error()
	if strings.Contains(strings.ToLower(err.Error()), "openrouter") {
		return "openrouter"
	}
	return "anthropic"
}

// extractQuoted 从 errstr 里挖出形如 `model "claude-foo"` 的引号内容；找不到时
// 返回 "?"。
func extractQuoted(errstr, after string) string {
	low := strings.ToLower(errstr)
	idx := strings.Index(low, strings.ToLower(after))
	if idx < 0 {
		return "?"
	}
	chunk := errstr[idx:]
	q1 := strings.IndexAny(chunk, `"'`)
	if q1 < 0 {
		return "?"
	}
	chunk = chunk[q1+1:]
	q2 := strings.IndexAny(chunk, `"'`)
	if q2 < 0 {
		return "?"
	}
	return chunk[:q2]
}

// isNetworkError 是否是网络错误。不严格——只需覆盖最常见的：DNS 找不到、连接拒绝、
// I/O timeout、context deadline。
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	s := strings.ToLower(err.Error())
	for _, kw := range []string{"connection refused", "no such host", "dial tcp", "i/o timeout", "tls handshake"} {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}
