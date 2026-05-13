// Package tools defines the GM agent's tool set and the Dispatcher that bridges
// each tool call to the rules engine and state store.
//
// 设计：Dispatcher 持有 store.Repository（可能是事务作用域）+ saveID + rng，
// 把 LLM 提供的原始 input JSON 解析后路由到具体 handler。LLM 永远看不到
// saveID / investigator_id 之外的上下文细节；这些由 orchestrator 在 RunTurn
// 内构造 Dispatcher 时绑定。
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/zhuzhenwu/whisperer/internal/agent"
	"github.com/zhuzhenwu/whisperer/internal/memory"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

// Dispatcher 把 tool 名字派发到具体 handler。一个 Dispatcher 绑定到单个 save 的
// 单回合执行；保存 saveID、当前 turn、rng。
//
// memory 与 npcAgent 是可选依赖：未注入时 npc_speak tool 会返回 isError=true。
// 这让早期里程碑 / 集成测试可以跳过向量库与 NPC 子代理依赖。
type Dispatcher struct {
	repo     *store.Repository
	saveID   string
	turn     int
	rng      *rand.Rand
	now      func() int64
	memory   *memory.Memory
	npcAgent *agent.NPCAgent
	scenario *scenario.Scenario
}

// New 构造一个 Dispatcher（不带 memory / NPC agent）。
//
//   - repo 通常是 store.RunTurn 内部传入的事务作用域 Repository。
//   - turn 是当前回合号（用于 add_event / mark_clue_found 等的默认值）。
func New(repo *store.Repository, saveID string, turn int, rng *rand.Rand) *Dispatcher {
	return &Dispatcher{
		repo:   repo,
		saveID: saveID,
		turn:   turn,
		rng:    rng,
		now:    func() int64 { return 0 },
	}
}

// WithMemory 注入向量记忆层。返回自身便于链式调用。
func (d *Dispatcher) WithMemory(m *memory.Memory) *Dispatcher {
	d.memory = m
	return d
}

// WithNPCAgent 注入 NPC 子代理。
func (d *Dispatcher) WithNPCAgent(a *agent.NPCAgent) *Dispatcher {
	d.npcAgent = a
	return d
}

// WithScenario 注入 effective scenario（含 variant patch）。npc_speak handler 用它读取
// NPC 的 secret 与 knowledge map 以注入 NPC 子代理 system prompt。可空——空时 NPC
// 不会得到 GM-only 的隐藏信息。
func (d *Dispatcher) WithScenario(s *scenario.Scenario) *Dispatcher {
	d.scenario = s
	return d
}

// SetClock 注入可控时钟（仅测试需要）。
func (d *Dispatcher) SetClock(now func() int64) { d.now = now }

// Tools 返回当前 Dispatcher 支持的全部 tool 定义，可直接传给 agent.GMAgent。
func (d *Dispatcher) Tools() []anthropic.ToolUnionParam {
	defs := allToolDefs()
	out := make([]anthropic.ToolUnionParam, len(defs))
	for i := range defs {
		t := defs[i]
		out[i] = anthropic.ToolUnionParam{OfTool: &t}
	}
	return out
}

// Dispatch 是 agent.ToolHandler 的实现。
//
// 任何 tool 错误都通过返回 (errStruct, true) 表达，永不返回 Go error。
func (d *Dispatcher) Dispatch(ctx context.Context, name string, inputRaw json.RawMessage) (any, bool) {
	h, ok := registry[name]
	if !ok {
		return errPayload(fmt.Sprintf("unknown tool: %s", name)), true
	}
	return h(ctx, d, inputRaw)
}

// handler 是单个 tool 的内部签名。
type handler func(ctx context.Context, d *Dispatcher, inputRaw json.RawMessage) (any, bool)

// registry 集中持有 tool name → handler 的映射；handlers_*.go 文件中通过 init() 注册。
var registry = map[string]handler{}

// register 在 handlers_*.go 的 init() 中调用。
func register(name string, h handler) {
	if _, exists := registry[name]; exists {
		panic("tools: duplicate registration: " + name)
	}
	registry[name] = h
}

// errPayload 是 tool 失败时统一返回的载荷。
func errPayload(msg string) map[string]any {
	return map[string]any{"error": msg}
}

// markAutosave 在关键 tool 成功执行后追加一条 autosave_checkpoint 事件。
//
// MVP 不实现"回滚到 checkpoint"功能，但事件流水中的标记便于:
//   - drift detector / 玩家复盘看到关键节点
//   - 未来引入回滚（克隆 SQLite 文件）时直接以 checkpoint 为锚点
//
// 失败静默：autosave 不应阻塞主操作。
func markAutosave(ctx context.Context, d *Dispatcher, label string) {
	related, _ := json.Marshal([]string{label})
	_, _ = d.repo.AppendEvent(ctx, store.Event{
		SaveID:              d.saveID,
		Turn:                d.turn,
		Type:                store.EventAutosaveCheckpoint,
		Description:         "autosave: " + label,
		RelatedEntitiesJSON: string(related),
	})
}

// decode 是 input JSON → typed struct 的简便包装。
func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	if len(raw) == 0 {
		return v, nil
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, fmt.Errorf("parse input: %w", err)
	}
	return v, nil
}
