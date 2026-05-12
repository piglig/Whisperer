package memory

import (
	"context"
	"fmt"

	chromem "github.com/philippgille/chromem-go"
)

// 集合名常量；每个 save 一个 chromem DB，不需要前缀。
const (
	CollectionEvents = "events"
	CollectionNPCs   = "npcs"
	CollectionClues  = "clues"
)

// Memory 是 GM 的长期记忆层。一个 Memory 对应一个 save。
type Memory struct {
	db       *chromem.DB
	embedder Embedder
	events   *chromem.Collection
	npcs     *chromem.Collection
	clues    *chromem.Collection

	persistDir string // "" 表示 in-memory
}

// New 构造一个 Memory。
//
//   - persistDir == "" 时使用进程内 DB，进程退出即丢失（适合测试 / 临时会话）
//   - 否则用 chromem.NewPersistentDB，与 SQLite 文件平行存放
func New(persistDir string, embedder Embedder) (*Memory, error) {
	if embedder == nil {
		return nil, fmt.Errorf("memory.New: embedder is required")
	}
	var db *chromem.DB
	if persistDir == "" {
		db = chromem.NewDB()
	} else {
		var err error
		db, err = chromem.NewPersistentDB(persistDir, false /*compress*/)
		if err != nil {
			return nil, fmt.Errorf("memory.New: persistent db: %w", err)
		}
	}

	getOrCreate := func(name string) (*chromem.Collection, error) {
		c, err := db.GetOrCreateCollection(name, nil, embedder)
		if err != nil {
			return nil, fmt.Errorf("memory.New: collection %s: %w", name, err)
		}
		return c, nil
	}
	events, err := getOrCreate(CollectionEvents)
	if err != nil {
		return nil, err
	}
	npcs, err := getOrCreate(CollectionNPCs)
	if err != nil {
		return nil, err
	}
	clues, err := getOrCreate(CollectionClues)
	if err != nil {
		return nil, err
	}

	return &Memory{
		db: db, embedder: embedder,
		events: events, npcs: npcs, clues: clues,
		persistDir: persistDir,
	}, nil
}

// Hit 是 Query* 的输出条目。
type Hit struct {
	ID      string            `json:"id"`
	Content string            `json:"content"`
	Meta    map[string]string `json:"meta,omitempty"`
	Score   float32           `json:"score"`
}

// upsert 把一条文档写入对应集合。chromem-go 的 AddDocuments 在 ID 重复时会覆盖。
func upsert(ctx context.Context, c *chromem.Collection, id, text string, meta map[string]string) error {
	if id == "" {
		return fmt.Errorf("memory: empty id")
	}
	return c.AddDocument(ctx, chromem.Document{
		ID:       id,
		Content:  text,
		Metadata: meta,
	})
}

// UpsertEvent 添加或覆盖一条事件文档。
func (m *Memory) UpsertEvent(ctx context.Context, id, text string, meta map[string]string) error {
	return upsert(ctx, m.events, id, text, meta)
}

// UpsertNPCProfile 添加或覆盖一个 NPC 人格档案文档。
func (m *Memory) UpsertNPCProfile(ctx context.Context, id, text string, meta map[string]string) error {
	return upsert(ctx, m.npcs, id, text, meta)
}

// UpsertClue 添加或覆盖一条线索文档。
func (m *Memory) UpsertClue(ctx context.Context, id, text string, meta map[string]string) error {
	return upsert(ctx, m.clues, id, text, meta)
}

// QueryEvents / QueryNPCs / QueryClues 返回按余弦相似度降序的 top-N。
//
// 集合为空时返回 nil 切片，不报错。
func (m *Memory) QueryEvents(ctx context.Context, q string, n int) ([]Hit, error) {
	return query(ctx, m.events, q, n)
}

func (m *Memory) QueryNPCs(ctx context.Context, q string, n int) ([]Hit, error) {
	return query(ctx, m.npcs, q, n)
}

func (m *Memory) QueryClues(ctx context.Context, q string, n int) ([]Hit, error) {
	return query(ctx, m.clues, q, n)
}

func query(ctx context.Context, c *chromem.Collection, q string, n int) ([]Hit, error) {
	if c.Count() == 0 {
		return nil, nil
	}
	if n <= 0 {
		n = 5
	}
	// chromem-go 在请求 N > 集合大小时会出错；夹紧。
	if n > c.Count() {
		n = c.Count()
	}
	results, err := c.Query(ctx, q, n, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("memory: query: %w", err)
	}
	hits := make([]Hit, len(results))
	for i, r := range results {
		hits[i] = Hit{
			ID:      r.ID,
			Content: r.Content,
			Meta:    r.Metadata,
			Score:   r.Similarity,
		}
	}
	return hits, nil
}

// Close 在持久化模式下 flush；in-memory 模式无操作。
//
// chromem-go 的 PersistentDB 在每次 Add* 时同步写盘，无需显式 flush；本方法仅作
// API 对称占位，未来引入 batch/flush 时可填实现。
func (m *Memory) Close() error { return nil }
