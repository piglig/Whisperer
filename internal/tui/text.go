package tui

import (
	"strings"

	"github.com/zhuzhenwu/whisperer/internal/store"
)

func summarizeError(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if text == "" {
		return "发生未知错误"
	}

	parts := []string{}
	if strings.Contains(text, "401") || strings.Contains(strings.ToLower(text), "unauthorized") {
		parts = append(parts, "模型调用失败", "401 Unauthorized")
		if strings.Contains(strings.ToLower(text), "x-api-key") {
			parts = append(parts, "invalid x-api-key")
		}
	} else if idx := strings.Index(text, ":"); idx > 0 && idx < 24 {
		parts = append(parts, strings.TrimSpace(text[:idx]))
		rest := strings.TrimSpace(text[idx+1:])
		if rest != "" {
			parts = append(parts, firstSentence(rest))
		}
	} else {
		parts = append(parts, firstSentence(text))
	}

	if requestID := extractRequestID(text); requestID != "" {
		parts = append(parts, "request "+requestID)
	}
	return strings.Join(parts, " · ")
}

func firstSentence(text string) string {
	for _, sep := range []string{". ", "。", "\n"} {
		if idx := strings.Index(text, sep); idx >= 0 {
			return strings.TrimSpace(text[:idx])
		}
	}
	return text
}

func extractRequestID(text string) string {
	for _, marker := range []string{"Request-ID:", "request_id\":\"", "request_id=", "req_"} {
		idx := strings.Index(text, marker)
		if idx < 0 {
			continue
		}
		if marker == "req_" {
			return readToken(text[idx:])
		}
		return readToken(text[idx+len(marker):])
	}
	return ""
}

func readToken(text string) string {
	text = strings.TrimLeft(text, " \t\"'")
	var b strings.Builder
	for _, r := range text {
		if r == '"' || r == '\'' || r == ')' || r == '}' || r == ',' || r == ' ' || r == '\t' || r == '\n' {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

func renderEntry(e logEntry) string {
	switch e.kind {
	case EntryGM:
		return gmStyle.Render("GM  ") + e.text
	case EntryPlayer:
		return playerStyle.Render("你  ") + e.text
	case EntrySystem:
		return systemStyle.Render("系统 ") + e.text
	case EntryError:
		return errorStyle.Render("错误 ") + e.text
	case EntryEnding:
		return endingStyle.Render("结局 ") + e.text
	}
	return e.text
}

func helpText() string {
	return strings.Join([]string{
		"命令",
		"/sheet                 显示调查员属性",
		"/inventory (/inv)      显示背包",
		"/time                  显示当前时段",
		"/talk <人物> <话>      指名对某人说话",
		"/all <话>              对全场说话",
		"/hint                  请求一个不剧透的提示",
		"/bind <name> <职业>    在结局后绑定新调查员到本剧本",
		"/help                  显示本帮助",
		"/quit (/exit /q)       退出",
	}, "\n")
}

func nonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func displayTimeOfDay(t store.TimeOfDay) string {
	switch t {
	case store.TimeMorning:
		return "清晨"
	case store.TimeAfternoon:
		return "午后"
	case store.TimeNight:
		return "夜晚"
	default:
		return "未知时段"
	}
}

func displayStage(stage string) string {
	switch strings.ToLower(strings.TrimSpace(stage)) {
	case "opening", "act1":
		return "开局"
	case "investigation", "act2":
		return "调查"
	case "confrontation", "act3":
		return "对峙"
	default:
		return nonEmpty(stage, "开局")
	}
}
