# 04 — Memory & RAG（W4）

## Goals

- 给 GM agent 提供按需检索的"长期记忆"：事件、NPC 档案、线索
- 让 NPC 在不同回合间保持口吻一致（基于历史 + persona）
- 引入轻量 NPCAgent（独立人格 prompt），通过 `npc_speak` tool 由 GM 调度
- 嵌入式向量库，零外部服务依赖（chromem-go 持久化文件 + SQLite 文件 + JSONL 平行存档）
- 测试不依赖外部 embedding API：用确定性 fake embedder

## Non-Goals

- 不引入独立向量服务（如 Qdrant / Weaviate）
- 不实装"语义触发器"（足够类似事件也算触发），W5 触发器再说
- 不做 cross-save 记忆共享

## 架构

```
internal/memory/
├── embedder.go       Embedder + Fake（hash → vector）
└── memory.go         Memory：3 collection + Upsert/Query
```

每个 save 拥有：
- 一个 SQLite 文件（W2，`saves(id)` 等）
- 一个 chromem-go 持久化目录（本 milestone）
- 一份 JSONL 调用日志（W3 起）

三者由 `save_id` 关联，不共享集合。

## Memory 接口

```go
type Memory struct { /* unexported chromem.DB + 3 collections */ }

func New(persistDir string, embedder chromem.EmbeddingFunc) (*Memory, error)
// persistDir == "" → in-memory（测试用）

func (m *Memory) UpsertEvent(ctx, id, text string, meta map[string]string) error
func (m *Memory) UpsertNPCProfile(ctx, id, text string, meta map[string]string) error
func (m *Memory) UpsertClue(ctx, id, text string, meta map[string]string) error

type Hit struct {
    ID       string
    Content  string
    Meta     map[string]string
    Score    float32
}

func (m *Memory) QueryEvents(ctx, query string, n int) ([]Hit, error)
func (m *Memory) QueryNPCs(ctx, query string, n int) ([]Hit, error)
func (m *Memory) QueryClues(ctx, query string, n int) ([]Hit, error)

func (m *Memory) Close() error
```

写入时机：
- 事件：每回合结束，由 orchestrator 把 `store.Event` 同步进 `events` collection
- NPC：剧本初始化（W5）+ 大变化时（关系剧变 / 死亡）由 orchestrator 重新 upsert profile
- 线索：`mark_clue_found` 触发时由 dispatcher 顺手 upsert 到 `clues`

读出时机：
- GM 主回合开始前由 orchestrator 检索 top-K 事件 + 当前地点 NPC 列表，注入 system prompt 上下文
- `npc_speak` 内部检索本 NPC 相关事件，作为 NPCAgent 的 history

## Embedder

`chromem.EmbeddingFunc` 是 `func(ctx, text) ([]float32, error)`。

- 测试 / dev 默认：`NewFakeEmbedder(dim int)` —— hash → 32 维稀疏 float（确定性，不依赖网络）
- 生产：调用方传入 `chromem.NewEmbeddingFuncOpenAI(...)` / `NewEmbeddingFuncVoyage(...)` 等
- 推荐：Voyage AI（Anthropic 推荐的 embedding 提供商）。chromem-go 当前没有 Voyage 原生工厂；可用 `NewEmbeddingFuncOpenAICompat` 指向 Voyage 兼容端点

## NPCAgent

```go
type NPCAgent struct { llm LLM; model anthropic.Model }

func NewNPC(llm LLM, model anthropic.Model) *NPCAgent

type NPCSpeakRequest struct {
    Persona       string  // 人格档案（store.NPC.Personality + KnowledgeJSON 摘要）
    RecentHistory string  // memory 检索得到的历史摘录
    Intent        string  // GM 提示，例如 "回应玩家关于黑暗仪式的询问，态度警惕"
    PlayerLine    string  // 玩家本次说的话（可空）
}

func (n *NPCAgent) Speak(ctx, req NPCSpeakRequest) (string, error)
```

- 单次 LLM 调用，无工具（NPC 不应直接修改世界状态——状态写入仍由 GM agent 负责）
- 用 Haiku（cheaper）
- 系统 prompt 模板 `prompts/npc_system.tmpl`：要求保持口吻一致、不打破 NPC 知识边界、单段台词输出

## npc_speak tool

```
input: { npc_id: string, intent: string, player_line?: string }
flow:
  1. d.repo.GetNPC(npc_id)            → personality + knowledge_json
  2. d.memory.QueryEvents("npc:"+id+" ...", 5) → recent history
  3. d.npcAgent.Speak(req)
  4. d.repo.AppendEvent({type:"narrative", description:"<NPC>: <line>", related:[npc_id]})
  5. return { dialogue: "...", npc_id: "..." }
```

GM 主回合的叙事完成后追加 NPC 台词；保留 SLA #3（NPC 一致性）所需的元数据。

## SLA 衔接

- SLA #3 NPC 一致性：检索同 NPC 历史事件 + persona 输入 NPCAgent，下游 LLM-judge 校验时也用同样数据
- SLA #4 物品守恒：检索 `clues`/`events`，校验器可拿到 destroyed_items 的语义信息
- SLA #6 失败即失败：narrative 拼接需要包含 NPC 台词与 GM 文本

## 测试

- `memory_test.go`：FakeEmbedder + in-memory，验证 Upsert / Query 排序符合 cosine 直觉
- `npc_test.go`：fakeLLM 返回固定文本，NPCAgent.Speak 行为确定
- `dispatcher_npc_test.go`：npc_speak 端到端：store 读 + memory 读 + NPCAgent + event 写

覆盖率门槛 ≥ 85%（与现有标准一致）。

## Open Questions

- ⏳ Embedder 是否要在 Open(ctx) 时缓存维度？目前每个 collection 自带，跳过
- ⏳ memory 持久化的版本格式：chromem-go 自己处理；W2 sqlc 推迟时一并回看
- ⏳ Voyage 集成的具体环境变量约定：留到 W6 接通真实 API 时定
