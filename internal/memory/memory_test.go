package memory

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestMemory(t *testing.T) *Memory {
	t.Helper()
	m, err := New("", NewFakeEmbedder(0))
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func TestNew_RequiresEmbedder(t *testing.T) {
	_, err := New("", nil)
	assert.ErrorContains(t, err, "embedder is required")
}

func TestUpsertAndQueryEvents_Ranking(t *testing.T) {
	m := newTestMemory(t)
	ctx := context.Background()

	require.NoError(t, m.UpsertEvent(ctx, "e1", "investigator finds blood-stained letter at harbor", map[string]string{"turn": "1"}))
	require.NoError(t, m.UpsertEvent(ctx, "e2", "Dr. Vance refuses to discuss the murder", map[string]string{"turn": "2"}))
	require.NoError(t, m.UpsertEvent(ctx, "e3", "the lighthouse beam sweeps the rocky shore", map[string]string{"turn": "3"}))

	hits, err := m.QueryEvents(ctx, "blood letter harbor", 2)
	require.NoError(t, err)
	require.Len(t, hits, 2)
	// 与查询最相关的应是 e1
	assert.Equal(t, "e1", hits[0].ID)
	assert.Greater(t, hits[0].Score, hits[1].Score)
	assert.NotEmpty(t, hits[0].Meta)
}

func TestQuery_EmptyCollection(t *testing.T) {
	m := newTestMemory(t)
	ctx := context.Background()
	hits, err := m.QueryEvents(ctx, "anything", 5)
	require.NoError(t, err)
	assert.Empty(t, hits)
}

func TestQuery_NClampedToCollectionSize(t *testing.T) {
	m := newTestMemory(t)
	ctx := context.Background()
	require.NoError(t, m.UpsertEvent(ctx, "e1", "alpha beta", nil))
	hits, err := m.QueryEvents(ctx, "alpha", 100)
	require.NoError(t, err)
	assert.Len(t, hits, 1)
}

func TestUpsert_OverwritesByID(t *testing.T) {
	m := newTestMemory(t)
	ctx := context.Background()
	require.NoError(t, m.UpsertNPCProfile(ctx, "n1", "stern doctor", nil))
	require.NoError(t, m.UpsertNPCProfile(ctx, "n1", "kindly priest", nil))
	hits, err := m.QueryNPCs(ctx, "priest", 1)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, "kindly priest", hits[0].Content)
}

func TestUpsert_EmptyIDRejected(t *testing.T) {
	m := newTestMemory(t)
	err := m.UpsertEvent(context.Background(), "", "x", nil)
	assert.Error(t, err)
}

func TestThreeCollectionsAreIsolated(t *testing.T) {
	m := newTestMemory(t)
	ctx := context.Background()
	require.NoError(t, m.UpsertEvent(ctx, "shared-id", "events bucket", nil))
	require.NoError(t, m.UpsertNPCProfile(ctx, "shared-id", "npcs bucket", nil))
	require.NoError(t, m.UpsertClue(ctx, "shared-id", "clues bucket", nil))

	e, err := m.QueryEvents(ctx, "events", 1)
	require.NoError(t, err)
	require.Len(t, e, 1)
	assert.Equal(t, "events bucket", e[0].Content)

	n, err := m.QueryNPCs(ctx, "npcs", 1)
	require.NoError(t, err)
	require.Len(t, n, 1)
	assert.Equal(t, "npcs bucket", n[0].Content)

	c, err := m.QueryClues(ctx, "clues", 1)
	require.NoError(t, err)
	require.Len(t, c, 1)
	assert.Equal(t, "clues bucket", c[0].Content)
}

func TestPersistentMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mem")

	m1, err := New(dir, NewFakeEmbedder(0))
	require.NoError(t, err)
	require.NoError(t, m1.UpsertEvent(context.Background(), "e1", "harbor at dusk", nil))
	require.NoError(t, m1.Close())

	m2, err := New(dir, NewFakeEmbedder(0))
	require.NoError(t, err)
	defer m2.Close()
	hits, err := m2.QueryEvents(context.Background(), "harbor dusk", 1)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, "e1", hits[0].ID)
}

func TestPersistent_BadDir(t *testing.T) {
	// 用一个被现有文件占据的路径作为 dir，触发 chromem 创建失败
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "not-a-dir")
	require.NoError(t, writeFile(filePath, "x"))
	_, err := New(filePath, NewFakeEmbedder(0))
	assert.Error(t, err)
}

func TestFakeEmbedder_EmptyInput(t *testing.T) {
	emb := NewFakeEmbedder(0)
	_, err := emb(context.Background(), "")
	assert.Error(t, err)
}

func TestFakeEmbedder_DeterministicAndNormalized(t *testing.T) {
	emb := NewFakeEmbedder(16)
	v1, err := emb(context.Background(), "harbor at dusk")
	require.NoError(t, err)
	v2, err := emb(context.Background(), "harbor at dusk")
	require.NoError(t, err)
	assert.Equal(t, v1, v2)

	var sumSq float32
	for _, x := range v1 {
		sumSq += x * x
	}
	// L2 ~ 1
	assert.InDelta(t, 1.0, sumSq, 0.01)
}

// writeFile 是测试辅助：避免引入 os.WriteFile 多余 import。
func writeFile(path, content string) error {
	return osWriteFile(path, content)
}
