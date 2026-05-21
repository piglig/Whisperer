# 02 — State Store（W2）

## Goals

- 给 orchestrator / agent 提供一套 thin、强类型的状态读写 API
- 单 SQLite 文件保存多个 save，运行时只依赖 `database/sql` + SQLite driver
- 提供 turn-scoped 事务接口，配合 SLA 校验失败时整回合回滚
- 单测覆盖 ≥ 85%（用 `:memory:` 库）

## Non-Goals

- 不做向量检索（W4 的 memory 包负责）
- 不做剧本数据加载（W5 的 scenario 包负责）
- 不做并发写：MVP 单玩家、单进程，SQLite 默认 WAL 即可
- 不引入 ORM；查询生成只用于稳定的高频 CRUD

## sqlc

- W2 曾推迟 sqlc；现在 repository 已超过 ADR 0002 的重新评估阈值
- `internal/store/queries/*.sql` + `internal/store/storesqlc` 覆盖全部 store 表
- `Repository` 仍是上层唯一 API，负责领域模型转换、错误映射和少量业务默认值
- 生成命令：`make sqlc`
- 决策记录：`specs/adr/0002-defer-sqlc.md`

## Schema（`internal/store/schema.sql`，embed）

字段命名遵循 SQLite 惯例：`*_json` 存原文 JSON、时间戳用 unix ms（INTEGER）。

### 表

- `saves(id, name, scenario_id, current_location_id, turn_count, created_at, updated_at)`
- `investigators(id, save_id, name, occupation, attrs_json, skills_json, hp, mp, san, inventory_json, active)`
- `npcs(id, save_id, name, personality, knowledge_json, relation_to_player, location_id, alive)`
- `locations(id, save_id, name, description, parent_id, connections_json, visited)`
- `items(id, save_id, name, description, owner_type, owner_id, properties_json, destroyed)`
- `clues(id, save_id, scenario_id, description, found, found_in_location_id, found_at_turn)`
- `events(id INTEGER PK AUTOINCREMENT, save_id, turn, type, description, related_entities_json, created_at)`

### 关键索引

- `idx_events_save_turn` on `events(save_id, turn)`
- `idx_npcs_save_location` on `npcs(save_id, location_id)`
- `idx_items_save_owner` on `items(save_id, owner_type, owner_id)`

### 外键

所有子表 `save_id REFERENCES saves(id) ON DELETE CASCADE`。`PRAGMA foreign_keys=ON` 在每次 Open 时执行。

### `items.destroyed` 与 SLA #4 物品守恒

物品销毁不删行，置 `destroyed=1`。SLA #4 校验器扫描 `destroyed=1` 的物品名，反向匹配 GM 文本。

## Interface

`internal/store/store.go` 暴露：

```go
type Store struct { db *sql.DB }

func Open(ctx context.Context, path string) (*Store, error)  // path == ":memory:" → 内存库
func (s *Store) Close() error
func (s *Store) Repo() *Repository
```

`internal/store/repository.go`：

```go
type Repository struct { /* 内部持有 db 或 tx，统一 querier 接口 */ }

// 写操作均接受 ctx；读操作返回零值结构体（不存在）+ sql.ErrNoRows 由 wrapper 包装为
// store.ErrNotFound（避免上层 import database/sql）
func (r *Repository) CreateSave(ctx, Save) error
func (r *Repository) GetSave(ctx, id) (Save, error)
func (r *Repository) ListSaves(ctx) ([]Save, error)
func (r *Repository) DeleteSave(ctx, id) error
func (r *Repository) UpdateSaveProgress(ctx, id, locationID string, turn int) error

func (r *Repository) UpsertInvestigator(ctx, Investigator) error
func (r *Repository) GetActiveInvestigator(ctx, saveID) (Investigator, error)
func (r *Repository) UpdateInvestigatorVitals(ctx, id string, hp, mp, san int) error
func (r *Repository) ListInvestigators(ctx, saveID) ([]Investigator, error)
func (r *Repository) DeactivateInvestigator(ctx, id) error  // 死亡或不定性疯狂时

func (r *Repository) UpsertNPC(ctx, NPC) error
func (r *Repository) GetNPC(ctx, id) (NPC, error)
func (r *Repository) ListNPCsAtLocation(ctx, saveID, locationID) ([]NPC, error)
func (r *Repository) UpdateNPCRelation(ctx, id, delta int) error
func (r *Repository) KillNPC(ctx, id) error

func (r *Repository) UpsertLocation(ctx, Location) error
func (r *Repository) GetLocation(ctx, id) (Location, error)
func (r *Repository) MarkLocationVisited(ctx, id) error

func (r *Repository) UpsertItem(ctx, Item) error
func (r *Repository) MoveItem(ctx, id, ownerType, ownerID string) error
func (r *Repository) DestroyItem(ctx, id) error
func (r *Repository) ListDestroyedItems(ctx, saveID) ([]Item, error)  // SLA #4 用

func (r *Repository) UpsertClue(ctx, Clue) error
func (r *Repository) MarkClueFound(ctx, id, locationID string, turn int) error
func (r *Repository) ListFoundClues(ctx, saveID) ([]Clue, error)

func (r *Repository) AppendEvent(ctx, Event) (int64, error)  // 返回 ID
func (r *Repository) ListEvents(ctx, saveID, fromTurn, toTurn int) ([]Event, error)

// turn-scoped 事务：fn 内通过传入的 *Repository 写入；fn 返回 error → 整回合回滚
func (s *Store) RunTurn(ctx context.Context, fn func(ctx context.Context, r *Repository) error) error
```

## 错误约定

- `store.ErrNotFound`：所有 Get* 在记录不存在时返回该错误（不直接暴露 `sql.ErrNoRows`）
- `store.ErrConflict`：Upsert 主键冲突且无法解决时返回（实际实现走 `INSERT ... ON CONFLICT DO UPDATE`，理论上不会出现）

## 测试

- `:memory:` 库 + `Store.Open(ctx, ":memory:")`
- 表驱动 + testify
- 关键场景：
  - CRUD round-trip 一致性
  - 外键级联删除（删 save 后子表清零）
  - `RunTurn` 失败 → 全部写入回滚
  - `RunTurn` 成功 → 写入持久化
  - JSON 字段读写不变形（属性/技能/知识）
  - Save 序列化 → 关闭 → 重开（文件 DB） → 状态完全相等

覆盖率 ≥ 85%。

## Open Questions

- ⏳ 是否对 `events.related_entities_json` 加 GIN-like 索引？SQLite 没有原生 JSON 索引，先按"前 N 个事件 + 向量库"组合检索；性能问题再说
- ⏳ Save 文件是否要内嵌 schema_version 字段？决定推迟到 W2 完成后回看
