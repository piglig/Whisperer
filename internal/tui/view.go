package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View 渲染整个界面。
func (m Model) View() string {
	if m.quitting {
		return "已退出。\n"
	}

	width := m.width
	if width <= 0 {
		width = 100
	}
	height := m.height
	if height <= 0 {
		height = 30
	}
	width = max(26, width)
	height = max(8, height)

	if width < 72 || height < 16 {
		return m.renderCompactShell(width, height)
	}
	return m.renderWorkbenchShell(width, height)
}

func (m Model) renderWorkbenchShell(width, height int) string {
	briefHeight := 2
	navHeight := 1
	composerHeight := 3
	contentHeight := max(6, height-1-briefHeight-navHeight-composerHeight)
	stage := m.renderMainStage(width, contentHeight)
	if m.overlay != overlayNone {
		stage = m.renderOverlay(width, contentHeight)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderSessionBar(width),
		m.renderBriefing(width, briefHeight),
		m.renderCommandDeck(width, navHeight),
		stage,
		m.renderComposer(width, composerHeight),
	)
}

func (m Model) renderCompactShell(width, height int) string {
	composerHeight := 2
	if height < 12 {
		composerHeight = 1
	}
	contentHeight := max(3, height-1-composerHeight)
	stage := m.renderTimeline(width, contentHeight)
	if m.overlay != overlayNone {
		stage = m.renderOverlay(width, contentHeight)
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderSessionBar(width),
		stage,
		m.renderComposer(width, composerHeight),
	)
}

func (m Model) renderSessionBar(width int) string {
	state := "待行动"
	if m.busy {
		state = "推演中"
	} else if m.endingShown {
		state = "已结局"
	}
	text := fmt.Sprintf("Whisperer 案件桌  回合 %d  %s    %s    %s",
		m.save.TurnCount,
		displayTimeOfDay(m.save.TimeOfDay),
		nonEmpty(m.inv.Name, "调查员"),
		state,
	)
	return sessionBarStyle.Width(width).Render(fitLine(text, width-2))
}

func (m Model) renderBriefing(width, height int) string {
	lines := []string{
		accentStyle.Render("任务简报") + "  " + m.briefLine(),
		accentStyle.Render("建议行动") + "  " + m.primarySuggestion(),
	}
	return fixedLines(lines, width, height)
}

func (m Model) renderCommandDeck(width, height int) string {
	guide := fmt.Sprintf("Ctrl+P 命令 · Shift+Tab 切换案件卡:%s · PgUp/PgDn 回看故事 · ? 帮助", m.activePanel.label())
	return fixedLines([]string{mutedStyle.Render(fitLine(guide, width))}, width, height)
}

func (m Model) renderComposer(width, height int) string {
	prompt := "› " + m.input.View()
	if m.busy {
		prompt = m.spinner.View() + " GM 正在推演回合..."
	}
	lines := []string{
		composerStyle.Render(fitLine(prompt, width)),
		mutedStyle.Render(fitLine(m.commandGuide(), width)),
	}
	return fixedLines(lines, width, height)
}

func (m Model) commandGuide() string {
	if strings.HasPrefix(m.input.Value(), "/talk ") {
		names := []string{}
		for _, npc := range m.npcs {
			if npc.ID != "" {
				names = append(names, npc.ID)
			}
		}
		if len(names) > 0 {
			return "Tab 补全目标 · " + strings.Join(names, " · ")
		}
	}
	if strings.TrimSpace(m.input.Value()) == "" && len(m.actionOptions()) > 0 {
		return "↑/↓ 选择行动 · 1-5 填入 · Enter 执行 · Ctrl+P 命令 · ? 帮助"
	}
	return "Enter 发送 · Tab 补全 · /hint 提示 · /talk 对话 · /sheet 状态 · /inv 背包"
}

func (m Model) renderOverlay(width, height int) string {
	switch m.overlay {
	case overlayCommands:
		return renderPinnedPane("命令面板", m.commandOverlayLines(), width, height)
	case overlayHelp:
		return renderPinnedPane("玩家帮助", m.helpOverlayLines(), width, height)
	default:
		return m.renderMainStage(width, height)
	}
}

func (m Model) commandOverlayLines() []string {
	return []string{
		accentStyle.Render("常用行动"),
		"  直接输入自然语言：检查门锁、追问范斯、翻找抽屉。",
		"  /talk <人物> <话>  指名对话，Tab 可补全当前地点人物。",
		"  /all <话>          对全场发言。",
		"",
		accentStyle.Render("调查工具"),
		"  /hint              请求不剧透提示。",
		"  /sheet             查看调查员属性。",
		"  /inv               查看背包。",
		"  /time              查看当前时段。",
		"",
		accentStyle.Render("会话"),
		"  /bind <名字> <职业>  结局后绑定新调查员。",
		"  /quit              退出。",
		"",
		mutedStyle.Render("Esc / Enter / Ctrl+P 关闭命令面板"),
	}
}

func (m Model) helpOverlayLines() []string {
	return []string{
		accentStyle.Render("桌面布局"),
		"  左侧是故事卷轴，保留玩家行动与 GM 叙事。",
		"  右侧案件卡按场景、人物、线索、裁定组织信息。",
		"  下方输入框只负责下一步行动，避免把命令说明挤进正文。",
		"",
		accentStyle.Render("快捷键"),
		"  Ctrl+P     打开命令面板。",
		"  Shift+Tab  切换案件卡标签页。",
		"  PgUp/PgDn  回看或回到最新故事。",
		"  Tab        补全斜杠命令或 NPC 目标。",
		"  Esc        退出；覆盖层打开时先关闭覆盖层。",
		"",
		accentStyle.Render("推荐玩法"),
		"  用自然语言描述意图，不必猜系统命令。",
		"  需要明确目标时使用 /talk；卡住时使用 /hint。",
		"",
		mutedStyle.Render("Esc / Enter / ? 关闭帮助"),
	}
}
