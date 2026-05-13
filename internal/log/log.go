// Package log 包装标准库 log/slog，提供：
//   - 文本（TUI / 开发）与 JSON（CI / 生产 / e2esmoke 机器解析）两种 handler
//   - 自动屏蔽 API key / token / secret 类字段
//   - 简洁的全局 SetDefault 入口
//
// 设计取舍：不引入 logrus / zerolog / zap 等第三方库。slog 在 stdlib，零依赖、
// 性能足够、与未来 OTEL bridge 兼容。
package log

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Format 是 handler 输出格式。
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

// New 构造 slog.Logger。w 为 nil 时写 os.Stderr。
//
//   - format == "json"：JSONHandler，便于 jq / 后续日志聚合
//   - 其他：TextHandler（默认）
//
// level 接受 "debug" / "info" / "warn" / "error"，不识别时退到 info。
func New(format, level string, w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stderr
	}
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(strings.ToLower(level))); err != nil {
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{
		Level:       lvl,
		ReplaceAttr: redactSensitive,
	}
	var h slog.Handler
	switch Format(strings.ToLower(format)) {
	case FormatJSON:
		h = slog.NewJSONHandler(w, opts)
	default:
		h = slog.NewTextHandler(w, opts)
	}
	return slog.New(h)
}

// SetDefault 把 logger 注入 slog.Default() —— 让任意 package 的 slog.Info / Error
// 走我们的配置，无需到处传 logger。
func SetDefault(l *slog.Logger) {
	slog.SetDefault(l)
}

// redactSensitive 是 slog 的 ReplaceAttr 钩子：把 key / token / secret 类字段值
// 替换为 "***"，避免误把 API key 落到日志或事件文件里。
//
// 触发条件：attr 的 key（不区分大小写）含 "key"、"token" 或 "secret"。
// 例外：log level / msg 等内置字段不受影响（它们的 key 是 "level" / "msg"）。
func redactSensitive(_ []string, a slog.Attr) slog.Attr {
	switch a.Key {
	case slog.TimeKey, slog.LevelKey, slog.MessageKey, slog.SourceKey:
		return a
	}
	k := strings.ToLower(a.Key)
	if strings.Contains(k, "key") || strings.Contains(k, "token") || strings.Contains(k, "secret") {
		return slog.String(a.Key, "***")
	}
	return a
}
