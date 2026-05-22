// Package orchestrator wires the GM agent, tool dispatcher, scenario engine,
// drift detector, SLA validator and persistence layer into one Turn loop.
//
// 一个 Orchestrator 绑定到一个 save。整个进程通常只活一个 Orchestrator——多 save
// 的切换由上层（CLI / 未来的 web）用新建实例完成。
package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/zhuzhenwu/whisperer/internal/agent"
	"github.com/zhuzhenwu/whisperer/internal/memory"
	"github.com/zhuzhenwu/whisperer/internal/orchestrator/sla"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

// Config 构造 Orchestrator 所需的依赖。
type Config struct {
	Store    *store.Store
	Memory   *memory.Memory
	Scenario *scenario.Scenario
	LLMGM    agent.LLM
	LLMNPC   agent.LLM // 可与 GM 共用同一个 client，仅模型常量不同
	SaveID   string
	RNG      *rand.Rand

	// AutosaveEvery：每 N 个回合刷一次 saves.updated_at（即软快照）。
	// 0 视为关闭。
	AutosaveEvery int

	// MaxSLARetries：SLA 失败时让 GM 重写 narrative 的次数（不 rollback Tx）。
	// 默认 1（即"原始 + 1 次重试"）。
	MaxSLARetries int

	// 模型可显式指定；空时用 agent.ModelGM / agent.ModelHelper。
	ModelGM  agent.Model
	ModelNPC agent.Model

	// Judge 是可选的 LLM-as-judge 实现，启用后会在结构化 SLA 之外做语义校验
	// （SLA #3 NPC 一致性 / #7 角色知识闭环）。每回合最多增加两次 Haiku 调用。
	// 缺省 nil 时 orchestrator 仅跑结构化 SLA（与 W6 行为一致）。
	Judge sla.Judge

	// VariantID 是本局所选的 variant id（来自 SelectVariant / SelectVariantByID）。
	// 仅作记账与结局上报使用——effective scenario（含 patch）通过 cfg.Scenario 传入。
	VariantID string

	// Meta 是跨周目玩家先验。可空（首次游玩即 nil）。
	Meta *scenario.MetaState

	// MetaPath 在通关时把更新后的 meta 写回磁盘；空字符串表示不持久化。
	MetaPath string

	// TraceDir 是 TurnTrace JSONL 落盘目录。每次会话写一份
	// <TraceDir>/<save_id>/<utc_ts>.jsonl，每行是一回合的 TraceEntry。
	// 空字符串 / "-" → 不落盘。
	TraceDir string
}

// Orchestrator 是 W6 的核心组件。
type Orchestrator struct {
	cfg         Config
	engine      *scenario.Engine
	detector    *scenario.Detector
	traceWriter *TraceWriter

	// 持续累计的对话历史（不持久化；进程重启后清空）
	history []agent.MessageParam
}

// New 构造 Orchestrator。
//
// 注意：Engine.Apply 不在 New 内调用——是否首次写入剧本数据由调用方决定（CLI 在
// 创建新 save 时显式调一次）。这样 New 可以在已加载的 save 上无副作用启动。
func New(cfg Config) (*Orchestrator, error) {
	if cfg.Store == nil {
		return nil, errors.New("orchestrator: Store is required")
	}
	if cfg.Scenario == nil {
		return nil, errors.New("orchestrator: Scenario is required")
	}
	if cfg.LLMGM == nil {
		return nil, errors.New("orchestrator: LLMGM is required")
	}
	if cfg.SaveID == "" {
		return nil, errors.New("orchestrator: SaveID is required")
	}
	if cfg.RNG == nil {
		cfg.RNG = rand.New(rand.NewPCG(1, 2))
	}
	if cfg.MaxSLARetries < 0 {
		cfg.MaxSLARetries = 0
	}
	if cfg.MaxSLARetries == 0 {
		cfg.MaxSLARetries = 1
	}
	if err := ValidateTraceDir(cfg.TraceDir); err != nil {
		return nil, fmt.Errorf("orchestrator: trace dir: %w", err)
	}
	o := &Orchestrator{cfg: cfg}
	o.engine = scenario.New(cfg.Scenario, cfg.Store.Repo(), cfg.Memory)
	o.detector = scenario.NewDetector(cfg.Scenario, cfg.Store.Repo())
	o.traceWriter = NewTraceWriter(cfg.TraceDir, cfg.SaveID)
	return o, nil
}

// TracePath 返回本次会话 TurnTrace JSONL 的文件路径；未启用时返回空字符串。
func (o *Orchestrator) TracePath() string {
	if o.traceWriter == nil {
		return ""
	}
	return o.traceWriter.Path()
}

// SaveID 返回当前绑定的 save。
func (o *Orchestrator) SaveID() string { return o.cfg.SaveID }

// Engine 暴露内部引擎，主要供 CLI 在新建 save 时调 Apply。
func (o *Orchestrator) Engine() *scenario.Engine { return o.engine }

// History 返回当前对话历史副本（只读）。
func (o *Orchestrator) History() []agent.MessageParam {
	out := make([]agent.MessageParam, len(o.history))
	copy(out, o.history)
	return out
}

// ResetHistory 清空对话历史。供调查员切换等场景使用。
func (o *Orchestrator) ResetHistory() { o.history = nil }

// VariantID 返回本局所选 variant id（可空）。
func (o *Orchestrator) VariantID() string { return o.cfg.VariantID }

// RecordCompletion 在剧本到达终局时把 variant + ending + 揭开的关键真相写入跨周目 meta，
// 然后落盘（若 MetaPath 非空）。重复调用幂等——MetaState.MarkCompletion 已去重。
func (o *Orchestrator) RecordCompletion(ctx context.Context, endingID string) error {
	if o.cfg.Meta == nil {
		return nil
	}
	repo := o.cfg.Store.Repo()
	clues, err := repo.ListFoundClues(ctx, o.cfg.SaveID)
	if err != nil {
		return fmt.Errorf("list found clues: %w", err)
	}
	truths := make([]string, 0, len(clues))
	for _, c := range clues {
		truths = append(truths, c.ID)
	}
	o.cfg.Meta.MarkCompletion(o.cfg.VariantID, endingID, truths)
	if o.cfg.MetaPath == "" {
		return nil
	}
	return scenario.SaveMeta(o.cfg.MetaPath, o.cfg.Meta)
}

// BindNewInvestigator 把一名新调查员接续到当前 save——先把旧的 active 调查员
// deactivate（如有），再 upsert 新调查员并设为 active；同时清空对话历史让
// 下回合不带上"前任"的对话遗留。
//
// 对应需求文档 F6.4：调查员死亡或不定性疯狂后，玩家可"绑定新调查员到本剧本进度"。
// 剧本世界状态保留（NPC / 地点 / 线索 / 事件流水），仅替换调查员实体。
func (o *Orchestrator) BindNewInvestigator(ctx context.Context, inv store.Investigator) error {
	repo := o.cfg.Store.Repo()
	if inv.ID == "" {
		return errors.New("BindNewInvestigator: ID is required")
	}
	inv.SaveID = o.cfg.SaveID
	inv.Active = true
	// 把所有旧调查员 deactivate（多数情况下只有一个）。
	existing, err := repo.ListInvestigators(ctx, o.cfg.SaveID)
	if err != nil {
		return fmt.Errorf("list existing investigators: %w", err)
	}
	for _, e := range existing {
		if e.Active && e.ID != inv.ID {
			if err := repo.DeactivateInvestigator(ctx, e.ID); err != nil {
				return fmt.Errorf("deactivate %s: %w", e.ID, err)
			}
		}
	}
	if err := repo.UpsertInvestigator(ctx, inv); err != nil {
		return fmt.Errorf("upsert new investigator: %w", err)
	}
	o.ResetHistory()
	return nil
}

// renderSystemPrompt 在每回合开始基于 store 当前快照构造 GM system prompt。
func (o *Orchestrator) renderSystemPrompt(ctx context.Context) (string, error) {
	repo := o.cfg.Store.Repo()
	sv, err := repo.GetSave(ctx, o.cfg.SaveID)
	if err != nil {
		return "", fmt.Errorf("get save: %w", err)
	}

	events, eventsErr := repo.ListEvents(ctx, o.cfg.SaveID, 0, 0)
	stage := scenario.NormalizeStage(o.cfg.Scenario, sv.Stage)
	if stage == "" {
		stage = scenario.DefaultStage
	}
	objective := scenario.ObjectiveForStage(o.cfg.Scenario, stage)

	scenarioContext := fmt.Sprintf("剧本: %s（%s, v%s）；当前回合 %d；时段 %s；当前阶段 %s",
		o.cfg.Scenario.Title, o.cfg.Scenario.ID, o.cfg.Scenario.Version, sv.TurnCount, sv.TimeOfDay, stage)
	if objective.Title != "" {
		scenarioContext += "；当前目标 " + objective.Title
	}

	pc := agent.PromptContext{
		ScenarioContext: scenarioContext,
		Truth:           scenario.RenderTruth(o.cfg.Scenario),
		NPCSecrets:      scenario.RenderNPCSecrets(o.cfg.Scenario),
		NPCKnowledge:    scenario.RenderNPCKnowledge(o.cfg.Scenario),
		ClueAtlas:       scenario.RenderClueAtlas(o.cfg.Scenario),
		PlayerGuidance:  scenario.RenderPlayerGuidance(o.cfg.Scenario),
	}
	if o.cfg.Meta != nil {
		pc.PlayerPrior = o.cfg.Meta.RenderForGM()
	}

	if inv, err := repo.GetActiveInvestigator(ctx, o.cfg.SaveID); err == nil {
		pc.InvestigatorBrief = fmt.Sprintf("%s（%s）· HP %d / MP %d / SAN %d",
			inv.Name, inv.Occupation, inv.HP, inv.MP, inv.SAN)
	}
	if sv.CurrentLocationID != "" {
		if loc, err := repo.GetLocation(ctx, sv.CurrentLocationID); err == nil {
			pc.LocationBrief = fmt.Sprintf("%s — %s", loc.Name, loc.Description)
		}
	}

	// 近期事件（最多 5 条）作为 RAG 的廉价替代；正式 RAG 走 memory.QueryEvents。
	if eventsErr == nil && len(events) > 0 {
		start := 0
		if len(events) > 5 {
			start = len(events) - 5
		}
		var b []byte
		for _, ev := range events[start:] {
			b = append(b, '-', ' ')
			b = append(b, []byte(ev.Description)...)
			b = append(b, '\n')
		}
		pc.RecentEvents = string(b)
	}

	pc.AvailableIDs = renderAvailableIDs(o.cfg.Scenario)

	return agent.RenderGMSystem(pc)
}

// renderAvailableIDs 按类目列出剧本中所有可被 tool 引用的实体 id 与人类名，
// 让 LLM 不必猜也不会拼错。
//
// 输出形如：
//
//	locations:
//	  - harbor (雾港码头)
//	  - pub (钨灯酒馆)
//	npcs:
//	  - vance (范斯医生)
//	clues:
//	  - blood_letter
//	items:
//	  - lantern (黄铜油灯)
func renderAvailableIDs(scn *scenario.Scenario) string {
	if scn == nil {
		return ""
	}
	var b []byte
	if len(scn.Locations) > 0 {
		b = append(b, "locations:\n"...)
		for _, l := range scn.Locations {
			b = append(b, "  - "...)
			b = append(b, l.ID...)
			if l.Name != "" {
				b = append(b, " ("...)
				b = append(b, l.Name...)
				b = append(b, ')')
			}
			b = append(b, '\n')
		}
	}
	if len(scn.NPCs) > 0 {
		b = append(b, "npcs:\n"...)
		for _, n := range scn.NPCs {
			b = append(b, "  - "...)
			b = append(b, n.ID...)
			if n.Name != "" {
				b = append(b, " ("...)
				b = append(b, n.Name...)
				b = append(b, ')')
			}
			b = append(b, '\n')
		}
	}
	if len(scn.Clues) > 0 {
		b = append(b, "clues:\n"...)
		for _, c := range scn.Clues {
			b = append(b, "  - "...)
			b = append(b, c.ID...)
			b = append(b, '\n')
		}
	}
	if len(scn.Items) > 0 {
		b = append(b, "items:\n"...)
		for _, it := range scn.Items {
			b = append(b, "  - "...)
			b = append(b, it.ID...)
			if it.Name != "" {
				b = append(b, " ("...)
				b = append(b, it.Name...)
				b = append(b, ')')
			}
			b = append(b, '\n')
		}
	}
	return string(b)
}

// snapshotForSLA 在 dispatcher 已经把 tools 的状态变更写入 Tx-scoped repo 之后调用，
// 抓取 Validator 需要的快照字段。
func snapshotForSLA(ctx context.Context, repo *store.Repository, saveID string) (sla.Snapshot, error) {
	snap := sla.Snapshot{InvestigatorActive: false}
	destroyed, err := repo.ListDestroyedItems(ctx, saveID)
	if err != nil {
		return snap, err
	}
	for _, it := range destroyed {
		if it.Name != "" {
			snap.DestroyedItemNames = append(snap.DestroyedItemNames, it.Name)
		}
	}
	if inv, err := repo.GetActiveInvestigator(ctx, saveID); err == nil {
		snap.InvestigatorActive = true
		snap.InvestigatorHP = inv.HP
		snap.InvestigatorSAN = inv.SAN
	}
	return snap, nil
}
