package tui

import "strings"

// command 是 / 命令解析结果。
type command struct {
	name string // 不含 /
	arg  string // 第一个参数（用于 /talk <NPC>）
	rest string // 其余字符串
}

// parseSlash 把以 "/" 开头的输入解析为命令。返回 ok=false 表示不是命令。
func parseSlash(text string) (command, bool) {
	if !strings.HasPrefix(text, "/") {
		return command{}, false
	}
	body := strings.TrimPrefix(text, "/")
	if body == "" {
		return command{}, false
	}
	// 拆 name + 余下
	parts := strings.SplitN(body, " ", 2)
	c := command{name: strings.ToLower(parts[0])}
	if len(parts) == 2 {
		rest := strings.TrimSpace(parts[1])
		// 对 /talk 这类需要"第一个 token 当 arg"的命令，再切一次
		switch c.name {
		case "talk":
			argParts := strings.SplitN(rest, " ", 2)
			c.arg = argParts[0]
			if len(argParts) == 2 {
				c.rest = strings.TrimSpace(argParts[1])
			}
		default:
			c.arg = rest
			c.rest = rest
		}
	}
	return c, true
}
