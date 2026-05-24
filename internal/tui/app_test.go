package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

// fakeRunner 假装一个 orchestrator，用于 Model 单测。
type fakeRunner struct {
	saveID string
	res    orchestrator.TurnResult
	err    error
	calls  []string
}

func (f *fakeRunner) RunTurn(_ context.Context, input string) (orchestrator.TurnResult, error) {
	f.calls = append(f.calls, input)
	return f.res, f.err
}

func (f *fakeRunner) SaveID() string { return f.saveID }

func newTestStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	id := uuid.NewString()
	require.NoError(t, s.Repo().CreateSave(ctx, store.Save{ID: id, Name: "x", ScenarioID: "fh"}))
	require.NoError(t, s.Repo().UpsertInvestigator(ctx, store.Investigator{
		ID: "inv", SaveID: id, Name: "Lyra", Occupation: "记者",
		AttrsJSON: "{}", SkillsJSON: "{}", InventoryJSON: `["pen"]`,
		HP: 12, MP: 10, SAN: 60, Active: true,
	}))
	require.NoError(t, s.Repo().UpsertLocation(ctx, store.Location{
		ID: "harbor", SaveID: id, Name: "雾港", Description: "x",
	}))
	require.NoError(t, s.Repo().UpsertNPC(ctx, store.NPC{
		ID: "vance", SaveID: id, Name: "范斯", Personality: "冷静",
		KnowledgeJSON: "{}", LocationID: "harbor", Alive: true,
	}))
	require.NoError(t, s.Repo().UpsertNPC(ctx, store.NPC{
		ID: "villager", SaveID: id, Name: "村民", Personality: "紧张",
		KnowledgeJSON: "{}", LocationID: "harbor", Alive: true,
	}))
	require.NoError(t, s.Repo().UpsertClue(ctx, store.Clue{
		ID: "cloth", SaveID: id, ScenarioID: "fh", Description: "湿布片",
		Found: true, FoundInLocationID: "harbor", FoundAtTurn: 1,
	}))
	require.NoError(t, s.Repo().UpsertItem(ctx, store.Item{
		ID: "lantern", SaveID: id, Name: "黄铜油灯", Description: "x",
		OwnerType: store.OwnerLocation, OwnerID: "harbor",
		PropertiesJSON: "{}",
	}))
	require.NoError(t, s.Repo().UpdateSaveProgress(ctx, id, "harbor", 0))
	return s, id
}

func testScenario() *scenario.Scenario {
	return &scenario.Scenario{
		Objectives: []scenario.Objective{
			{
				Stage: "opening",
				Title: "确认露西失踪前最后去了哪里",
				Steps: []string{"查看码头公告栏", "找海莲娜问话"},
			},
			{
				Stage: "investigation",
				Title: "把潮汐、账册和出诊记录串起来",
				Steps: []string{"追问账册", "争取玛丽莎信任"},
			},
		},
		Threats: []scenario.Threat{
			{
				ID:   "cloth_seen",
				Name: "布片风险",
				States: []scenario.ThreatState{
					{ID: "quiet", Label: "平稳"},
					{
						ID:       "marked",
						Label:    "已出现",
						Severity: 2,
						When:     scenario.Condition{ClueFound: "cloth"},
					},
				},
			},
		},
		Locations: []scenario.SLocation{
			{
				ID:          "harbor",
				Name:        "雾港",
				Description: "x",
				Leads: []string{
					"查看公告栏上的失踪启事",
					"沿着退潮线寻找布料",
					"询问码头工人最近是否见过露西",
				},
			},
		},
		NPCs: []scenario.SNPC{
			{
				ID:       "vance",
				Name:     "范斯",
				Location: "harbor",
				DialogueOptions: []scenario.DialogueOption{
					{
						ID:     "ask_lucy",
						Label:  "问露西是否找过他",
						Prompt: "询问范斯，露西失踪前是否来找过他。",
						Stages: []string{"opening"},
					},
				},
			},
		},
		Items: []scenario.SItem{
			{
				ID:          "lantern",
				Name:        "黄铜油灯",
				Description: "x",
				OwnerType:   "location",
				OwnerID:     "harbor",
				Actions: []scenario.ItemAction{
					{
						ID:         "take",
						Label:      "拿起黄铜油灯",
						Prompt:     "拿起黄铜油灯并检查灯芯。",
						Stages:     []string{"opening"},
						Locations:  []string{"harbor"},
						OwnerTypes: []string{"location"},
					},
				},
			},
		},
	}
}

func TestModel_OpeningRendered(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeRunner{saveID: id}
	m := New(context.Background(), r, st, "开场白", testScenario())
	v := m.View()
	assert.Contains(t, v, "开场白")
	assert.Contains(t, v, "Whisperer")
	assert.Contains(t, v, "任务简报")
	assert.Contains(t, v, "案件板 / 可行动项")
	assert.Contains(t, v, "行动流")
	assert.Contains(t, v, "案件卡")
	assert.Contains(t, v, "阶段 开局")
	assert.Contains(t, v, "当前目标")
	assert.Contains(t, v, "当前重点")
	assert.Contains(t, v, "确认露西失踪前最后去了哪里")
	assert.Contains(t, v, "查看码头公告栏")
}

func TestModel_SnapshotMsgPopulatesState(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeRunner{saveID: id}
	m := New(context.Background(), r, st, "")
	cmd := m.loadSnapshotCmd()
	msg := cmd()
	updated, _ := m.Update(msg)
	mm := updated.(Model)
	assert.Equal(t, "Lyra", mm.inv.Name)
	assert.Equal(t, "雾港", mm.location.Name)
	assert.Len(t, mm.npcs, 2)
	assert.Len(t, mm.clues, 1)
	assert.Equal(t, store.TimeMorning, mm.save.TimeOfDay)
}

func TestModel_HandleSlashHelp(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeRunner{saveID: id}
	m := New(context.Background(), r, st, "")
	m.input.SetValue("/help")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	last := mm.log[len(mm.log)-1]
	assert.Equal(t, EntrySystem, last.kind)
	assert.Contains(t, last.text, "/sheet")
}

func TestModel_HandleSlashSheetAndInventory(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeRunner{saveID: id}
	m := New(context.Background(), r, st, "")
	// 注入 inventory
	cmd := m.loadSnapshotCmd()
	updated, _ := m.Update(cmd())
	mm := updated.(Model)

	mm.input.SetValue("/sheet")
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = updated.(Model)
	assert.Contains(t, mm.log[len(mm.log)-1].text, "Lyra")

	mm.input.SetValue("/inv")
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = updated.(Model)
	assert.Contains(t, mm.log[len(mm.log)-1].text, "pen")
}

func TestModel_HandleSlashUnknown(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	m.input.SetValue("/unknown")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	assert.Equal(t, EntryError, mm.log[len(mm.log)-1].kind)
}

func TestModel_HandleSlashTalkRequiresArg(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	m.input.SetValue("/talk")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	assert.Equal(t, EntryError, mm.log[len(mm.log)-1].kind)
}

func TestModel_PlainInputTriggersRunTurn(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeRunner{
		saveID: id,
		res: orchestrator.TurnResult{
			Narrative: "你环顾四周。",
			Drift:     scenario.DriftOK,
		},
	}
	m := New(context.Background(), r, st, "")
	m.input.SetValue("look around")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	assert.True(t, mm.busy)

	// 直接调一次 runTurnCmd 拿到 turnDoneMsg，避开 tea.Batch 包装。
	msg := mm.runTurnCmd("look around")()
	updated, _ = mm.Update(msg)
	mm = updated.(Model)
	assert.False(t, mm.busy)
	assert.Contains(t, mm.log[len(mm.log)-1].text, "你环顾四周")
	assert.NotEmpty(t, r.calls)
}

func TestModel_RunTurnError(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeRunner{saveID: id, err: errors.New("boom")}
	m := New(context.Background(), r, st, "")
	updated, _ := m.Update(turnDoneMsg{err: errors.New("boom")})
	mm := updated.(Model)
	assert.Equal(t, EntryError, mm.log[len(mm.log)-1].kind)
	_ = r
}

func TestModel_TurnResultEnding(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	res := orchestrator.TurnResult{
		Narrative: "终。",
		Ending:    &scenario.Ending{ID: "victim_dies", Kind: "failure", Description: "失败"},
		Drift:     scenario.DriftHard,
	}
	updated, _ := m.Update(turnDoneMsg{res: res})
	mm := updated.(Model)
	assert.True(t, mm.endingShown)
	assert.Contains(t, mm.drift, "硬偏离")
	hasEnd := false
	for _, e := range mm.log {
		if e.kind == EntryEnding {
			hasEnd = true
		}
	}
	assert.True(t, hasEnd)
}

func TestFormatCaseReport(t *testing.T) {
	report := &scenario.CaseReport{
		Ending:         scenario.Ending{ID: "solved", Kind: "success", Description: "结案"},
		EvidenceStatus: "关键证据完整",
		CulpritName:    "范斯医生",
		FoundKeyClues:  []scenario.ReportClue{{ID: "ledger", Description: "账册"}},
		MissingKeyClues: []scenario.ReportClue{
			{
				ID:          "reef_carvings",
				Description: "礁洞拓片",
				Leads: []scenario.ClueLead{
					{Kind: "trigger", TriggerID: "reef_cave_open", Hint: "条件：已触发：lighthouse_storm + 第 14 回合之后"},
					{Kind: "location", LocationID: "reef_cave", Hint: "去 礁洞"},
				},
			},
		},
		NPCOutcomes:  []scenario.NPCOutcome{{ID: "anna", Name: "安娜", Alive: false}},
		Threats:      []scenario.ThreatStatus{{Name: "安娜危险", StateLabel: "失踪", Severity: 4}},
		TruthSummary: "本局真相摘要",
	}

	out := formatCaseReport(report)
	assert.Contains(t, out, "结案评估")
	assert.Contains(t, out, "本局真凶")
	assert.Contains(t, out, "账册")
	assert.Contains(t, out, "礁洞拓片")
	assert.Contains(t, out, "下次试试")
	assert.Contains(t, out, "去 礁洞")
	assert.Contains(t, out, "安娜")
	assert.Contains(t, out, "风险结算")
	assert.Contains(t, out, "本局真相摘要")
}

func TestFormatTurnSummary(t *testing.T) {
	out := formatTurnSummary(orchestrator.TurnSummary{
		LocationChange: &orchestrator.ValueChange{From: "雾港码头", To: "钨灯酒馆"},
		TimeChange:     &orchestrator.ValueChange{From: string(store.TimeMorning), To: string(store.TimeAfternoon)},
		StageChange:    &orchestrator.ValueChange{From: "opening", To: "investigation"},
		NewClues:       []orchestrator.SummaryClue{{ID: "ledger", Description: "账册"}},
		NPCChanges: []orchestrator.SummaryNPC{{
			ID: "marisa", Name: "玛丽莎", RelationFrom: 5, RelationTo: 10,
		}},
		ThreatChanges: []orchestrator.SummaryThreat{{Name: "安娜危险", From: "被盯上", To: "高危"}},
	})

	assert.Contains(t, out, "回合摘要")
	assert.Contains(t, out, "雾港码头 → 钨灯酒馆")
	assert.Contains(t, out, "清晨 → 午后")
	assert.Contains(t, out, "开局 → 调查")
	assert.Contains(t, out, "账册")
	assert.Contains(t, out, "玛丽莎")
	assert.Contains(t, out, "安娜危险")
}

func TestFormatTurnReview(t *testing.T) {
	out := formatTurnReview(orchestrator.TurnResult{
		Action: orchestrator.PlayerAction{Text: "查看公告栏"},
		Decision: orchestrator.TurnDecision{
			Intent: orchestrator.IntentInvestigate,
			Action: orchestrator.PlayerAction{Text: "查看公告栏"},
			Mechanics: []orchestrator.DecisionAction{{
				Tool:    "roll_skill",
				Success: true,
				Detail:  `{"skill_name":"Spot Hidden","skill_value":60,"difficulty":"regular","roll":47,"threshold":60,"success":true}`,
			}},
			Checks: []orchestrator.DecisionCheck{{
				Code:    "action_guard",
				Passed:  false,
				Message: "这个调查行动不属于当前地点。",
			}},
		},
		Summary: orchestrator.TurnSummary{
			NewClues: []orchestrator.SummaryClue{{ID: "cloth", Description: "湿布片"}},
		},
	})

	assert.Contains(t, out, "回合复盘")
	assert.Contains(t, out, "行动：调查 · 查看公告栏")
	assert.Contains(t, out, "新线索：湿布片")
	assert.Contains(t, out, "未执行：这个调查行动不属于当前地点。")
	assert.Contains(t, out, "裁定：Spot Hidden")
}

func TestModel_ActionBoardIsPrimarySurface(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "开场白", testScenario())
	updated, _ := m.Update(m.loadSnapshotCmd()())
	m = updated.(Model)
	m.width = 120
	m.height = 30

	v := m.View()

	assert.Contains(t, v, "案件板 / 可行动项")
	assert.Contains(t, v, "选择行动")
	assert.Contains(t, v, "Enter 执行")
	assert.Contains(t, v, "1. 调查 · 查看公告栏上的失踪启事")
}

func TestFormatTurnRuling(t *testing.T) {
	out := formatTurnRuling(orchestrator.TurnDecision{
		Mechanics: []orchestrator.DecisionAction{
			{
				Tool:    "roll_skill",
				Success: true,
				Detail:  `{"skill_name":"Spot Hidden","skill_value":60,"difficulty":"regular","roll":47,"threshold":60,"success":true,"degree":"regular_success"}`,
			},
			{
				Tool:    "sanity_check",
				Success: true,
				Detail:  `{"roll":72,"threshold":60,"success":false,"loss":2,"new_san":58,"triggered_indefinite_insanity":false}`,
			},
		},
		Checks: []orchestrator.DecisionCheck{
			{Code: "rules_applied", Passed: true},
			{Code: "roll_missing", Passed: false, Message: "叙事提到检定但没有掷骰"},
		},
	})

	assert.Contains(t, out, "裁定记录")
	assert.Contains(t, out, "Spot Hidden：60 → 47，成功")
	assert.Contains(t, out, "SAN：60 → 72，失败，损失 2，当前 SAN 58")
	assert.Contains(t, out, "系统拦截：叙事提到检定但没有掷骰")
}

func TestModel_QuitOnCtrlC(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	mm := updated.(Model)
	assert.True(t, mm.quitting)
}

func TestModel_BusyIgnoresEnter(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	m.busy = true
	m.input.SetValue("hi")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	assert.True(t, mm.busy)
	// 仍然 busy；不应触发新一回合
}

func TestModel_WindowResize(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	mm := updated.(Model)
	assert.Equal(t, 100, mm.width)
	assert.Equal(t, 40, mm.height)
}

func TestModel_ViewKeepsTerminalHeightWithLongHistory(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "开场白")
	updated, _ := m.Update(m.loadSnapshotCmd()())
	m = updated.(Model)
	m.width = 82
	m.height = 18
	for i := 0; i < 30; i++ {
		m.log = append(m.log,
			logEntry{kind: EntryPlayer, text: "我继续调查码头仓库里一段很长很长很长的描述，用来模拟玩家连续输入命令。"},
			logEntry{kind: EntryGM, text: "GM 返回一段很长很长很长的叙事文本，包含状态、地点、线索和后续行动建议。"},
			logEntry{kind: EntryError, text: "模型调用失败: 这是一段很长很长很长的错误信息，不能把右侧面板撑破。"},
		)
	}

	v := m.View()

	assert.LessOrEqual(t, lipgloss.Height(v), 18)
	for _, line := range strings.Split(v, "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 82)
	}
}

func TestModel_ViewCollapsesNarrativeHistory(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	m.width = 100
	m.height = 28
	for i := 0; i < 18; i++ {
		m.log = append(m.log, logEntry{kind: EntryGM, text: "叙事段落"})
	}

	v := m.View()

	assert.Contains(t, v, "之前 4 条行动已折叠")
}

func TestModel_ViewSummarizesProviderErrors(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	m.width = 100
	m.height = 28
	raw := `回合失败: gm respond: agent: LLM call failed at iter 1: POST "https://api.anthropic.com/v1/messages": 401 Unauthorized {"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"},"request_id":"req_011CbFaau68bKZM"}`
	m.log = append(m.log, logEntry{kind: EntryError, text: raw})

	v := m.View()
	summary := summarizeError(raw)

	assert.Contains(t, summary, "模型调用失败")
	assert.Contains(t, summary, "401 Unauthorized")
	assert.Contains(t, summary, "invalid x-api-key")
	assert.Contains(t, v, "模型调用失败")
	assert.NotContains(t, v, "https://api.anthropic.com")
}

func TestModel_ViewUsesWorkbenchLayoutWithCaseRail(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "开场白")
	updated, _ := m.Update(m.loadSnapshotCmd()())
	m = updated.(Model)
	m.width = 120
	m.height = 30
	m.input.SetValue("/talk ")

	v := m.View()

	assert.Contains(t, v, "Whisperer 案件桌")
	assert.Contains(t, v, "任务简报")
	assert.Contains(t, v, "建议行动")
	assert.Contains(t, v, "案件板 / 可行动项")
	assert.Contains(t, v, "行动流")
	assert.Contains(t, v, "案件卡")
	assert.Contains(t, v, "地点")
	assert.Contains(t, v, "在场人物")
	assert.Contains(t, v, "/talk vance")
	assert.Contains(t, v, "Tab 补全目标")
	assert.NotContains(t, v, "case:fh")
}

func TestModel_CommandAndHelpOverlays(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "开场白")
	m.width = 100
	m.height = 28

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	mm := updated.(Model)
	assert.Equal(t, overlayCommands, mm.overlay)
	assert.Contains(t, mm.View(), "命令面板")
	assert.Contains(t, mm.View(), "指名对话")

	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = updated.(Model)
	assert.Equal(t, overlayNone, mm.overlay)

	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	mm = updated.(Model)
	assert.Equal(t, overlayHelp, mm.overlay)
	assert.Contains(t, mm.View(), "玩家帮助")
	assert.Contains(t, mm.View(), "Shift+Tab")
}

func TestModel_CaseRailTabsCycle(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "开场白", testScenario())
	updated, _ := m.Update(m.loadSnapshotCmd()())
	m = updated.(Model)
	m.width = 120
	m.height = 30

	assert.Contains(t, m.View(), "地点")

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = updated.(Model)
	assert.Equal(t, panelPeople, m.activePanel)
	assert.Contains(t, m.View(), "人物")
	assert.Contains(t, m.View(), "/talk vance")

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = updated.(Model)
	assert.Equal(t, panelClues, m.activePanel)
	assert.Contains(t, m.View(), "线索")
	assert.Contains(t, m.View(), "湿布片")
}

func TestModel_SuggestedActionsFillAndSubmit(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "", testScenario())
	updated, _ := m.Update(m.loadSnapshotCmd()())
	mm := updated.(Model)
	mm.width = 120
	mm.height = 36

	view := mm.View()
	assert.Contains(t, view, "风险")
	assert.Contains(t, view, "布片风险")
	assert.Contains(t, view, "可选行动")
	assert.Contains(t, view, "1. 调查 · 查看公告栏上的失踪启事")
	assert.Contains(t, view, "4. 使用 · 使用黄铜油灯")
	assert.Contains(t, view, "↑/↓ 选择行动")
	assert.Contains(t, strings.Join(mm.actionOptions(), "\n"), "询问范斯：问露西是否找过他")
	encoded := mm.selectedAction()
	action, ok := orchestrator.DecodePlayerAction(encoded)
	require.True(t, ok)
	assert.Equal(t, "lead", action.Source.Kind)
	assert.Equal(t, "harbor:0", action.Source.ID)

	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	mm = updated.(Model)
	assert.Equal(t, "沿着退潮线寻找布料", mm.input.Value())

	mm.input.SetValue("")
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	mm = updated.(Model)
	assert.Equal(t, "拿起黄铜油灯并检查灯芯。", mm.input.Value())

	mm.input.SetValue("")
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	mm = updated.(Model)
	assert.Equal(t, "询问范斯，露西失踪前是否来找过他。", mm.input.Value())

	mm.input.SetValue("")
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyUp})
	mm = updated.(Model)
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyUp})
	mm = updated.(Model)
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = updated.(Model)
	assert.True(t, mm.busy)
	require.NotEmpty(t, mm.log)
	assert.Equal(t, EntryPlayer, mm.log[len(mm.log)-1].kind)
	assert.Equal(t, "询问码头工人最近是否见过露西", mm.log[len(mm.log)-1].text)
}

func TestModel_StoryScrollsWithPageKeys(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	m.width = 100
	m.height = 18
	for i := 0; i < 24; i++ {
		m.log = append(m.log, logEntry{kind: EntryGM, text: "叙事段落"})
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	mm := updated.(Model)
	assert.Greater(t, mm.storyOffset, 0)
	assert.Contains(t, mm.View(), "↑ 更早内容")

	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	mm = updated.(Model)
	assert.Zero(t, mm.storyOffset)
}

func TestModel_ViewHidesCommandHintOnShortScreens(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "开场白")
	m.width = 36
	m.height = 10

	v := m.View()

	assert.LessOrEqual(t, lipgloss.Height(v), 10)
	assert.NotContains(t, v, "Enter 发送")
	for _, line := range strings.Split(v, "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 36)
	}
}

func TestModel_TimeCommand(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	cmd := m.loadSnapshotCmd()
	updated, _ := m.Update(cmd())
	mm := updated.(Model)
	mm.input.SetValue("/time")
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = updated.(Model)
	assert.Contains(t, mm.log[len(mm.log)-1].text, "清晨")
}

func TestModel_AllCommandRunsTurn(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeRunner{saveID: id, res: orchestrator.TurnResult{Narrative: "众人沉默。"}}
	m := New(context.Background(), r, st, "")
	m.input.SetValue("/all 我是侦探")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	assert.True(t, mm.busy)
	// 直接跑 runTurnCmd 还原 RunTurn 调用
	msg := mm.runTurnCmd("[all] 我是侦探")()
	updated, _ = mm.Update(msg)
	mm = updated.(Model)
	require.NotEmpty(t, r.calls)
	assert.Contains(t, r.calls[0], "[all]")
	assert.Contains(t, mm.log[len(mm.log)-1].text, "众人沉默")
	assert.NotContains(t, mm.log[0].text, "[all]")
	assert.Contains(t, mm.log[0].text, "对在场所有人说")
}

func TestModel_TalkCommandRunsTurn(t *testing.T) {
	st, id := newTestStore(t)
	r := &fakeRunner{saveID: id, res: orchestrator.TurnResult{Narrative: "范斯说话。"}}
	m := New(context.Background(), r, st, "")
	m.input.SetValue("/talk vance 你看到什么了？")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	assert.True(t, mm.busy)
	msg := mm.runTurnCmd("[talk:vance] 你看到什么了？")()
	updated, _ = mm.Update(msg)
	require.NotEmpty(t, r.calls)
	assert.Contains(t, r.calls[0], "[talk:vance]")
	assert.NotContains(t, mm.log[0].text, "[talk:vance]")
	assert.Contains(t, mm.log[0].text, "对 vance 说")
	_ = updated
}

func TestModel_QuitCommand(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	m.input.SetValue("/quit")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	assert.True(t, mm.quitting)
	assert.Contains(t, mm.View(), "已退出")
}

func TestModel_TabCompletesCommand(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	m.input.SetValue("/hi")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	mm := updated.(Model)

	assert.Equal(t, "/hint", mm.input.Value())
}

func TestModel_TabCompletesTalkNPC(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	updated, _ := m.Update(m.loadSnapshotCmd()())
	mm := updated.(Model)
	mm.input.SetValue("/talk va")

	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyTab})
	mm = updated.(Model)

	assert.Equal(t, "/talk vance", mm.input.Value())
}

func TestModel_TabCyclesTalkNPCs(t *testing.T) {
	st, id := newTestStore(t)
	m := New(context.Background(), &fakeRunner{saveID: id}, st, "")
	updated, _ := m.Update(m.loadSnapshotCmd()())
	mm := updated.(Model)
	mm.input.SetValue("/talk v")

	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyTab})
	mm = updated.(Model)
	first := mm.input.Value()
	mm.input.SetValue("/talk v")
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyTab})
	mm = updated.(Model)
	second := mm.input.Value()

	assert.NotEqual(t, first, second)
	assert.Contains(t, []string{first, second}, "/talk vance")
	assert.Contains(t, []string{first, second}, "/talk villager")
}
