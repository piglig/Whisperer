package scenario

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeta_LoadMissingReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	m, err := LoadMeta(filepath.Join(dir, "nope.json"))
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, 0, m.PlayCount)
}

func TestMeta_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.json")
	m := &MetaState{}
	m.MarkCompletion("vance_pact", "pact_broken", []string{"sacrifice_chamber", "reef_carvings"})
	require.NoError(t, SaveMeta(path, m))

	loaded, err := LoadMeta(path)
	require.NoError(t, err)
	assert.Equal(t, 1, loaded.PlayCount)
	assert.Contains(t, loaded.CompletedVariants, "vance_pact")
	assert.Contains(t, loaded.CompletedEndings, "pact_broken")
	assert.Contains(t, loaded.DiscoveredTruths, "sacrifice_chamber")
}

func TestMeta_MarkCompletionDeduplicates(t *testing.T) {
	m := &MetaState{}
	m.MarkCompletion("v1", "e1", []string{"t1", "t2"})
	m.MarkCompletion("v1", "e1", []string{"t1", "t3"})
	assert.Equal(t, 2, m.PlayCount)
	assert.Equal(t, []string{"v1"}, m.CompletedVariants)
	assert.Equal(t, []string{"e1"}, m.CompletedEndings)
	assert.ElementsMatch(t, []string{"t1", "t2", "t3"}, m.DiscoveredTruths)
}

func TestMeta_RenderForGM(t *testing.T) {
	m := &MetaState{}
	assert.Equal(t, "", m.RenderForGM(), "fresh meta renders empty")

	m.MarkCompletion("vance_pact", "solved", []string{"sacrifice_chamber"})
	out := m.RenderForGM()
	assert.Contains(t, out, "1")
	assert.Contains(t, out, "vance_pact")
	assert.Contains(t, out, "似曾相识")
	assert.Contains(t, out, "不可让 NPC 直接说出")
}
