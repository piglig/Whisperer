package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrate 应用所有未应用的迁移。
//
// 引入 goose 之前的 DB（v0.1–v0.3.x）没有 goose_db_version 表，但 schema 已经处于
// 各种历史状态：
//   - v0.1 / v0.2 schema：缺 `saves.variant_id` 列
//   - v0.3.x schema：含 `saves.variant_id` 列（直接由旧的 schema.sql 一次性 apply）
//
// 我们先尝试用一个 hard-coded 检测把已有 schema "stamp" 到对应的 goose version；如
// 果是空 DB，goose 会从 0 开始顺序应用全部迁移。
func migrate(ctx context.Context, db *sql.DB) error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("goose set dialect: %w", err)
	}

	if err := bootstrapPreGoose(ctx, db); err != nil {
		return fmt.Errorf("pre-goose bootstrap: %w", err)
	}

	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}

// bootstrapPreGoose 处理 goose 引入前已经存在的 DB：
//
//   - 没有 saves 表 → 全新 DB，什么都不做（goose 从 version 0 开始）
//   - 有 saves 表但没有 goose_db_version → pre-goose 旧 DB；按照已有 schema 的
//     完整度把 goose 版本 stamp 到对应位置（避免 goose 重新 apply 已存在的 DDL）
//   - 已有 goose_db_version → 一切正常，goose UP 自处理
func bootstrapPreGoose(ctx context.Context, db *sql.DB) error {
	// goose 自己的版本表是否存在？
	hasGoose, err := tableExists(ctx, db, "goose_db_version")
	if err != nil {
		return err
	}
	if hasGoose {
		return nil
	}

	hasSaves, err := tableExists(ctx, db, "saves")
	if err != nil {
		return err
	}
	if !hasSaves {
		// 全新 DB；goose 会跑全部迁移。
		return nil
	}

	// pre-goose DB。判定停在哪个版本：
	hasVariant, err := columnExists(ctx, db, "saves", "variant_id")
	if err != nil {
		return err
	}

	// goose v3 需要先确保 version 表存在。EnsureDBVersion 会建表并写入 0；
	// 然后我们用 SetVersion-equivalent 直接插入想停的版本号。
	if _, err := goose.EnsureDBVersionContext(ctx, db); err != nil {
		return fmt.Errorf("ensure goose version table: %w", err)
	}

	target := int64(1) // pre-variant_id 旧 schema：已经等价于 0001 init 的产物
	if hasVariant {
		target = 2 // 已经含 variant_id：等价于 0002 已应用
	}
	if err := stampGooseVersion(ctx, db, target); err != nil {
		return fmt.Errorf("stamp goose version=%d: %w", target, err)
	}
	return nil
}

func tableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	row := db.QueryRowContext(ctx,
		`SELECT 1 FROM sqlite_master WHERE type='table' AND name = ? LIMIT 1`, name)
	var x int
	err := row.Scan(&x)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func columnExists(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info(%q)`, table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, rows.Close()
		}
	}
	return false, rows.Err()
}

// stampGooseVersion 把 goose_db_version 直接置为指定版本（不实际跑 SQL）。
// 用于 pre-goose 已存在的 schema "停在历史版本"。
func stampGooseVersion(ctx context.Context, db *sql.DB, version int64) error {
	for v := int64(1); v <= version; v++ {
		// goose 的 version 表 schema：(id, version_id, is_applied, tstamp)
		if _, err := db.ExecContext(ctx,
			`INSERT INTO goose_db_version (version_id, is_applied, tstamp) VALUES (?, 1, CURRENT_TIMESTAMP)`,
			v,
		); err != nil {
			return err
		}
	}
	return nil
}
