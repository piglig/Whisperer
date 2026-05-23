package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/zhuzhenwu/whisperer/internal/agent"
	"github.com/zhuzhenwu/whisperer/internal/orchestrator/sla"
	"github.com/zhuzhenwu/whisperer/internal/orchestrator/tools"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
	"github.com/zhuzhenwu/whisperer/internal/telemetry"
)

var errSLAAttemptFailed = errors.New("sla attempt failed")

// TurnResult 是 RunTurn 的结构化输出。TUI / CLI 据此渲染。
type TurnResult struct {
	Narrative string                  `json:"narrative"`
	Trace     agent.TurnTrace         `json:"trace"`
	Fired     []scenario.FiredTrigger `json:"fired,omitempty"`
	Drift     scenario.DriftStatus    `json:"drift"`
	Ending    *scenario.Ending        `json:"ending,omitempty"`
	Report    *scenario.CaseReport    `json:"report,omitempty"`
	Decision  TurnDecision            `json:"decision"`
	Action    PlayerAction            `json:"action"`
	Summary   TurnSummary             `json:"summary,omitempty"`
	SLAReport sla.Report              `json:"sla_report"`
	Save      store.Save              `json:"save"`
}

// RunTurn 执行一次完整回合：管线见 specs/06-orchestrator-and-tui.md §RunTurn。
//
// 出错语义：返回 error 时 Tx 已 rollback，调用方应把错误显示给玩家但不要修改
// 进程内 history（保持一致性）。
func (o *Orchestrator) RunTurn(ctx context.Context, userInput string) (TurnResult, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "whisperer.turn")
	defer span.End()

	span.SetAttributes(
		attribute.String("save_id", o.cfg.SaveID),
		attribute.String("variant_id", o.cfg.VariantID),
	)

	preSave, err := o.cfg.Store.Repo().GetSave(ctx, o.cfg.SaveID)
	if err != nil {
		span.SetStatus(codes.Error, "get save")
		span.RecordError(err)
		return TurnResult{}, fmt.Errorf("get save: %w", err)
	}
	turnNumber := preSave.TurnCount + 1
	span.SetAttributes(attribute.Int("turn", turnNumber))
	preSnapshot, err := captureTurnSnapshot(ctx, o.cfg.Store.Repo(), o.cfg.SaveID, o.cfg.Scenario)
	if err != nil {
		span.SetStatus(codes.Error, "capture pre-turn snapshot")
		span.RecordError(err)
		return TurnResult{}, fmt.Errorf("pre-turn snapshot: %w", err)
	}
	playerAction := parsePlayerAction(ctx, o.cfg.Store.Repo(), o.cfg.SaveID, o.cfg.Scenario, userInput)
	span.SetAttributes(attribute.String("turn.intent", string(playerAction.Kind)))
	guard := guardPlayerAction(ctx, o.cfg.Store.Repo(), o.cfg.SaveID, o.cfg.Scenario, playerAction)
	if !guard.Allowed {
		decision := buildGuardDecision(playerAction, guard)
		return TurnResult{
			Narrative: guardNarrative(guard),
			Decision:  decision,
			Action:    guard.NormalizedAction,
			SLAReport: sla.Report{Passed: true},
			Save:      preSave,
		}, nil
	}
	playerAction = guard.NormalizedAction
	gmInput := buildGMUserInput(playerAction, guard)

	systemPrompt, err := o.renderSystemPrompt(ctx)
	if err != nil {
		span.SetStatus(codes.Error, "render system prompt")
		span.RecordError(err)
		return TurnResult{}, err
	}

	var (
		trace      agent.TurnTrace
		newHistory []agent.MessageParam
		report     sla.Report
		firedList  []scenario.FiredTrigger
		drift      scenario.DriftStatus
		ending     *scenario.Ending
		caseReport *scenario.CaseReport
		decision   TurnDecision
		summary    TurnSummary
		finalSave  store.Save
	)

	maxAttempts := o.cfg.MaxSLARetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	attemptInput := gmInput

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		isLastAttempt := attempt == maxAttempts
		txErr := o.cfg.Store.RunTurn(ctx, func(ctx context.Context, repo *store.Repository) error {
			dispatcher := tools.New(repo, o.cfg.SaveID, turnNumber, o.cfg.RNG)
			if o.cfg.Memory != nil {
				dispatcher.WithMemory(o.cfg.Memory)
			}
			if o.cfg.LLMNPC != nil {
				dispatcher.WithNPCAgent(agent.NewNPC(o.cfg.LLMNPC, o.cfg.ModelNPC))
			}
			dispatcher.WithScenario(o.cfg.Scenario)

			gm, err := agent.New(agent.Config{
				LLM:          o.cfg.LLMGM,
				Model:        o.cfg.ModelGM,
				SystemPrompt: systemPrompt,
				Tools:        dispatcher.Tools(),
				Handler:      dispatcher.Dispatch,
			})
			if err != nil {
				return fmt.Errorf("build gm agent: %w", err)
			}

			t, hist, err := gm.Respond(ctx, o.history, attemptInput)
			if err != nil {
				return fmt.Errorf("gm respond: %w", err)
			}
			trace, newHistory = t, hist

			// 结构化 SLA：基于 Tx 内最新快照检查；违规 → 回滚本次 attempt，
			// 下一次把违规说明注入 user input 重新生成，避免保留失败 attempt 的 tool 写入。
			// 启用 Judge 时同步跑 LLM-as-judge（SLA #3 #7）。
			snap, err := snapshotForSLA(ctx, repo, o.cfg.SaveID)
			if err != nil {
				return fmt.Errorf("sla snapshot: %w", err)
			}
			validator := sla.New(snap)
			if o.cfg.Judge != nil {
				jctx, jerr := buildJudgeContext(ctx, repo, o.cfg.SaveID, trace)
				if jerr != nil {
					return fmt.Errorf("judge context: %w", jerr)
				}
				report = validator.CheckWithJudge(ctx, trace, o.cfg.Judge, jctx)
			} else {
				report = validator.Check(trace)
			}
			if !report.Passed && !isLastAttempt {
				attemptInput = gmInput + "\n\n" + buildSLAFeedback(report)
				return errSLAAttemptFailed
			}

			// 触发器 + drift + endings 仍在同 Tx 内执行。注意：必须用 tx-scoped repo，
			// 否则会与外层 Tx 抢同一条 SQLite 连接（SetMaxOpenConns(1)）→ 死锁。
			txEngine := scenario.New(o.cfg.Scenario, repo, o.cfg.Memory)
			txDetector := scenario.NewDetector(o.cfg.Scenario, repo)

			fired, err := txEngine.Evaluate(ctx, o.cfg.SaveID)
			if err != nil {
				return fmt.Errorf("trigger evaluate: %w", err)
			}
			firedList = fired

			// 落 GM 主回合 narrative 为一条 narrative event（便于复盘 + drift 兜底）。
			if trace.Narrative != "" {
				_, _ = repo.AppendEvent(ctx, store.Event{
					SaveID:      o.cfg.SaveID,
					Turn:        turnNumber,
					Type:        store.EventNarrative,
					Description: trace.Narrative,
				})
			}

			// 推进 turn count。
			//
			// 关键：必须先读 Tx 内最新的 save 状态再 UpdateSaveProgress——dispatcher 可能在
			// LLM 工具循环里调过 transition_location，把 current_location_id 改到新地点。
			// 直接用 preSave.CurrentLocationID 会把刚写入的新 location 覆盖回去（W6 真实
			// e2e 暴露的 bug）。
			curSave, err := repo.GetSave(ctx, o.cfg.SaveID)
			if err != nil {
				return fmt.Errorf("re-read save before advance: %w", err)
			}
			if err := repo.UpdateSaveProgress(ctx, o.cfg.SaveID, curSave.CurrentLocationID, turnNumber); err != nil {
				return fmt.Errorf("advance turn: %w", err)
			}

			// drift / ending
			dst, err := txDetector.Tick(ctx, o.cfg.SaveID, turnNumber)
			if err != nil {
				return fmt.Errorf("drift tick: %w", err)
			}
			drift = dst

			end, err := txEngine.CheckEndings(ctx, o.cfg.SaveID)
			if err != nil {
				return fmt.Errorf("check endings: %w", err)
			}
			// SLA #8 强制收束：投查员死亡 / SAN=0 → 用 fallback ending。
			if end == nil && report.EndingForced {
				end = &scenario.Ending{
					ID:          "investigator_lost",
					Kind:        "failure",
					Description: "调查员意识断裂——再无法继续追查这桩谜团。",
				}
			}
			ending = end

			sv, err := repo.GetSave(ctx, o.cfg.SaveID)
			if err != nil {
				return fmt.Errorf("re-read save: %w", err)
			}
			finalSave = sv
			if ending != nil {
				caseReport, err = buildCaseReport(ctx, repo, o.cfg.SaveID, o.cfg.Scenario, finalSave, ending, o.cfg.VariantID)
				if err != nil {
					return fmt.Errorf("case report: %w", err)
				}
			}
			postSnapshot, err := captureTurnSnapshot(ctx, repo, o.cfg.SaveID, o.cfg.Scenario)
			if err != nil {
				return fmt.Errorf("post-turn snapshot: %w", err)
			}
			summary = buildTurnSummary(preSnapshot, postSnapshot, firedList)
			decision = buildTurnDecision(playerAction, trace, summary, report)
			return nil
		})
		if txErr == nil {
			break
		}
		if errors.Is(txErr, errSLAAttemptFailed) {
			continue
		}
		span.SetStatus(codes.Error, "turn tx failed")
		span.RecordError(txErr)
		return TurnResult{}, txErr
	}

	// 提交后：history 替换、memory 异步落地（dispatcher 已写过对话事件；这里把
	// GM narrative 也落到 events 集合便于后续 RAG）。
	o.history = newHistory
	if o.cfg.Memory != nil && trace.Narrative != "" {
		_ = o.cfg.Memory.UpsertEvent(ctx,
			fmt.Sprintf("turn-%d-narrative", turnNumber),
			trace.Narrative,
			map[string]string{"turn": fmt.Sprintf("%d", turnNumber), "kind": "gm_narrative"},
		)
	}

	result := TurnResult{
		Narrative: trace.Narrative,
		Trace:     trace,
		Fired:     firedList,
		Drift:     drift,
		Ending:    ending,
		Report:    caseReport,
		Decision:  decision,
		Action:    playerAction,
		Summary:   summary,
		SLAReport: report,
		Save:      finalSave,
	}

	span.SetAttributes(
		attribute.Int("trace.iterations", trace.Iterations),
		attribute.Int64("trace.input_tokens", trace.InputTokens),
		attribute.Int64("trace.output_tokens", trace.OutputTokens),
		attribute.Float64("trace.cost_usd", trace.TotalCostUSD),
		attribute.Int("trace.tool_calls", len(trace.ToolCalls)),
		attribute.Int("scenario.fired_triggers", len(firedList)),
		attribute.String("scenario.drift", drift.String()),
		attribute.Bool("sla.passed", report.Passed),
	)
	if ending != nil {
		span.SetAttributes(
			attribute.String("scenario.ending_id", ending.ID),
			attribute.String("scenario.ending_kind", ending.Kind),
		)
	}

	// 写 JSONL trace（disabled 时是 no-op）。失败不影响本回合返回值——观察层
	// 故障不能让玩家重输。
	if err := o.traceWriter.Append(TraceEntry{
		SaveID:     o.cfg.SaveID,
		VariantID:  o.cfg.VariantID,
		TurnNumber: turnNumber,
		Result:     result,
		UserInput:  userInput,
	}); err != nil {
		slog.Warn("trace write failed",
			"err", err,
			"path", o.traceWriter.Path(),
			"turn", turnNumber)
	}

	return result, nil
}

// buildJudgeContext 抓出 LLM-judge 需要的上下文：每个 npc_speak 涉及的 NPC persona +
// 近期台词；调查员当前的技能 JSON 与职业字段。
func buildJudgeContext(ctx context.Context, repo *store.Repository, saveID string, trace agent.TurnTrace) (sla.JudgeContext, error) {
	jc := sla.JudgeContext{
		NPCPersonas:    map[string]string{},
		NPCRecentLines: map[string][]string{},
	}
	for _, tc := range trace.ToolCalls {
		if tc.Name != "npc_speak" || tc.IsError {
			continue
		}
		var out struct {
			NPCID string `json:"npc_id"`
		}
		if err := json.Unmarshal(tc.Output, &out); err != nil || out.NPCID == "" {
			continue
		}
		if _, ok := jc.NPCPersonas[out.NPCID]; ok {
			continue
		}
		npc, err := repo.GetNPC(ctx, out.NPCID)
		if err != nil {
			continue
		}
		jc.NPCPersonas[out.NPCID] = npc.Personality
		// 近期台词：从 events 中找该 NPC 相关的 narrative（最多 5 条）。
		events, _ := repo.ListEvents(ctx, saveID, 0, 0)
		var lines []string
		for _, ev := range events {
			if ev.Type == store.EventNarrative && bytesContains(ev.RelatedEntitiesJSON, out.NPCID) {
				lines = append(lines, ev.Description)
			}
		}
		if len(lines) > 5 {
			lines = lines[len(lines)-5:]
		}
		jc.NPCRecentLines[out.NPCID] = lines
	}
	if inv, err := repo.GetActiveInvestigator(ctx, saveID); err == nil {
		jc.InvestigatorSkillsJSON = inv.SkillsJSON
		jc.InvestigatorOccupation = inv.Occupation
	}
	return jc, nil
}

// bytesContains 是 strings.Contains 的零拷贝替代——related_entities_json 是字符串列表。
func bytesContains(s, sub string) bool {
	return s != "" && len(sub) > 0 && (s == sub || (len(s) > len(sub) && stringIndex(s, sub) >= 0))
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// buildSLAFeedback 把 SLA Report 包成给 LLM 的 user-side 反馈。
func buildSLAFeedback(r sla.Report) string {
	if len(r.Violations) == 0 {
		return ""
	}
	type item struct {
		Code       sla.Code `json:"code"`
		Message    string   `json:"message"`
		Suggestion string   `json:"suggestion,omitempty"`
	}
	items := make([]item, len(r.Violations))
	for i, v := range r.Violations {
		items[i] = item{Code: v.Code, Message: v.Message, Suggestion: v.Suggestion}
	}
	b, _ := json.Marshal(items)
	return "<sla_violation>\n" +
		"刚才的输出违反了规则。请仅重写本回合的 narrative，使之与已经执行的 tool 调用一致；不要再触发新的状态变更。问题清单：\n" +
		string(b) + "\n</sla_violation>"
}
