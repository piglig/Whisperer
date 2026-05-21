package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrate_FreshDB 在空 DB 上跑迁移后应能正常 Insert/Select。
func TestMigrate_FreshDB(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, ":memory:")
	require.NoError(t, err)
	defer st.Close()

	// 验证 goose_db_version 表存在且有版本号
	var version int64
	err = st.db.QueryRowContext(ctx,
		`SELECT MAX(version_id) FROM goose_db_version`).Scan(&version)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, version, int64(2), "应至少跑到 0002")

	// CRUD 烟雾
	r := st.Repo()
	require.NoError(t, r.CreateSave(ctx, Save{
		ID: "s1", Name: "x", ScenarioID: "fog_harbor", VariantID: "vance_executes",
	}))
	got, err := r.GetSave(ctx, "s1")
	require.NoError(t, err)
	assert.Equal(t, "vance_executes", got.VariantID)
}
