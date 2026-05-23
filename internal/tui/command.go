package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zhuzhenwu/whisperer/internal/investigator"
)

func (m Model) handleCommand(c command) (Model, tea.Cmd) {
	switch c.name {
	case "quit", "exit", "q":
		m.quitting = true
		return m, tea.Quit
	case "help":
		m.log = append(m.log, logEntry{kind: EntrySystem, text: helpText()})
	case "sheet":
		m.log = append(m.log, logEntry{kind: EntrySystem, text: m.renderSheet()})
	case "inventory", "inv":
		m.log = append(m.log, logEntry{kind: EntrySystem, text: m.renderInventory()})
	case "time":
		m.log = append(m.log, logEntry{kind: EntrySystem, text: "当前时段：" + displayTimeOfDay(m.save.TimeOfDay)})
	case "bind":
		return m.handleBind(c)
	case "hint":
		// /hint 走 RunTurn，把 user-input 改为系统提示语，让 GM 用环境/NPC 暗示拉回主线，
		// 不直接给答案。GM prompt（gm_system.tmpl §Granularity）已要求"高价值发现先问"，
		// 这里通过显式输入触发暗示。
		text := "[hint] 玩家请求一个不剧透的小提示：请用环境细节或在场 NPC 的轻微暗示，引导玩家关注主线。不要直接给出答案。"
		m.log = append(m.log, logEntry{kind: EntrySystem, text: "（已请求 GM 提示）"})
		m.busy = true
		return m, tea.Batch(m.runTurnCmd(text), m.spinner.Tick)
	case "talk":
		// /talk <人物>：把后续输入当作"对该人物说话"，MVP 实现为 prefix 注入并立即提交。
		if c.arg == "" {
			m.log = append(m.log, logEntry{kind: EntryError, text: "用法: /talk <人物> <你想说的话>"})
			return m, nil
		}
		text := fmt.Sprintf("[talk:%s] %s", c.arg, c.rest)
		m.log = append(m.log, logEntry{kind: EntryPlayer, text: m.talkDisplayText(c.arg, c.rest)})
		m.busy = true
		return m, tea.Batch(m.runTurnCmd(text), m.spinner.Tick)
	case "all":
		text := "[all] " + c.rest
		m.log = append(m.log, logEntry{kind: EntryPlayer, text: "对在场所有人说：" + c.rest})
		m.busy = true
		return m, tea.Batch(m.runTurnCmd(text), m.spinner.Tick)
	default:
		m.log = append(m.log, logEntry{kind: EntryError, text: "未知命令: /" + c.name + "（/help 查看可用命令）"})
	}
	return m, nil
}

func (m Model) talkDisplayText(target, speech string) string {
	name := target
	for _, npc := range m.npcs {
		if strings.EqualFold(npc.ID, target) || strings.EqualFold(npc.Name, target) {
			name = nonEmpty(npc.Name, target)
			break
		}
	}
	if strings.TrimSpace(speech) == "" {
		return "准备与 " + name + " 交谈。"
	}
	return "对 " + name + " 说：" + speech
}

// handleBind 实现 /bind <name> <occupation>。
//
// 仅在 ending 已展示时启用，避免玩家在游戏中途意外替换调查员。属性走默认值，
// 真正的"分步建卡流程"留给后续 milestone（需求文档 F1.4）。
func (m Model) handleBind(c command) (Model, tea.Cmd) {
	if !m.endingShown {
		m.log = append(m.log, logEntry{kind: EntryError, text: "/bind 仅在剧本结束后可用"})
		return m, nil
	}
	binder, ok := m.runner.(Binder)
	if !ok {
		m.log = append(m.log, logEntry{kind: EntryError, text: "/bind 在当前 runner 上不可用"})
		return m, nil
	}
	if c.arg == "" {
		m.log = append(m.log, logEntry{kind: EntryError, text: "用法: /bind <名字> <职业>"})
		return m, nil
	}
	parts := strings.SplitN(c.rest, " ", 2)
	name := parts[0]
	occupation := "调查员"
	if len(parts) == 2 {
		occupation = strings.TrimSpace(parts[1])
	}
	inv := investigator.NewQuick(m.runner.SaveID(), name, occupation)
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := binder.BindNewInvestigator(ctx, inv); err != nil {
		m.log = append(m.log, logEntry{kind: EntryError, text: "绑定失败: " + err.Error()})
		return m, nil
	}
	m.log = append(m.log, logEntry{
		kind: EntrySystem,
		text: fmt.Sprintf("已绑定新调查员: %s（%s）。剧本进度保留。", name, occupation),
	})
	m.endingShown = false
	return m, m.loadSnapshotCmd()
}

func (m Model) renderSheet() string {
	if m.inv.Name == "" {
		return "（尚未加载调查员）"
	}
	return fmt.Sprintf(
		"调查员: %s（%s）\nHP %d · MP %d · SAN %d\n属性: %s\n技能: %s",
		m.inv.Name, m.inv.Occupation, m.inv.HP, m.inv.MP, m.inv.SAN,
		m.inv.AttrsJSON, m.inv.SkillsJSON,
	)
}

func (m Model) renderInventory() string {
	if m.inv.InventoryJSON == "" {
		return "（背包为空）"
	}
	return "背包: " + m.inv.InventoryJSON
}
