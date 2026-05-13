package store

import (
	"context"
	"database/sql"
	"path/filepath"
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

// TestMigrate_PreGoose_OldSchema 模拟"v0.1–v0.2 旧 DB"：手工建一份不含 variant_id
// 也不含 goose_db_version 的 schema，再调 Open。期望 bootstrap stamp 到 version=1，
// 然后 0002 ALTER 把 variant_id 加上。
func TestMigrate_PreGoose_OldSchema(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "old.db")

	// 用裸 sql.Open 建一个"v0.1 风格"的 DB
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		CREATE TABLE saves (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			scenario_id TEXT NOT NULL,
			current_location_id TEXT,
			turn_count INTEGER NOT NULL DEFAULT 0,
			time_of_day TEXT NOT NULL DEFAULT 'morning' CHECK (time_of_day IN ('morning','afternoon','night')),
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);
	`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// 现在用正式 Open 打开 → 应自动迁移
	st, err := Open(ctx, dbPath)
	require.NoError(t, err)
	defer st.Close()

	// variant_id 列应该已经被加上
	exists, err := columnExists(ctx, st.db, "saves", "variant_id")
	require.NoError(t, err)
	assert.True(t, exists, "variant_id 列应在 0002 后存在")

	// goose 版本应该为 2
	var version int64
	err = st.db.QueryRowContext(ctx,
		`SELECT MAX(version_id) FROM goose_db_version`).Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, int64(2), version)
}

// TestMigrate_PreGoose_NewerSchema 模拟"v0.3.x 旧 DB"：已经含 variant_id，但仍无
// goose_db_version。期望 bootstrap stamp 到 version=2，0002 不会再次执行（否则
// ALTER 会报 duplicate column）。
func TestMigrate_PreGoose_NewerSchema(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "v03.db")

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		CREATE TABLE saves (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			scenario_id TEXT NOT NULL,
			variant_id TEXT NOT NULL DEFAULT '',
			current_location_id TEXT,
			turn_count INTEGER NOT NULL DEFAULT 0,
			time_of_day TEXT NOT NULL DEFAULT 'morning' CHECK (time_of_day IN ('morning','afternoon','night')),
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);
	`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	st, err := Open(ctx, dbPath)
	require.NoError(t, err, "应成功 bootstrap 已有 variant_id 的 DB 而不重复 ALTER")
	defer st.Close()

	var version int64
	err = st.db.QueryRowContext(ctx,
		`SELECT MAX(version_id) FROM goose_db_version`).Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, int64(2), version, "应 stamp 到最新版本")
}
