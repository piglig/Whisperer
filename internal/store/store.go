// Package store implements the SQLite-backed state store for Whisperer.
//
// 单 SQLite 文件即一个 save；所有 entity 通过 save_id 关联。Schema 由 goose
// 在 Open 时按 internal/store/migrations/*.sql 顺序应用，老 DB 自动升级。
//
// 错误约定：所有 Get* 在记录不存在时返回 ErrNotFound，避免上层 import database/sql。
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动
)

// 标准错误。
var (
	ErrNotFound = errors.New("store: not found")
	ErrConflict = errors.New("store: conflict")
)

// Store 是 Whisperer 状态层入口。一个 *Store 对应一个 SQLite 库文件（或 :memory:）。
type Store struct {
	db *sql.DB
}

// Open 打开（或创建）一个 SQLite 文件并应用 schema。
//
// path == ":memory:" 时使用进程内内存库（仅用于测试）。
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := path
	if path == ":memory:" {
		// modernc.org/sqlite 的 in-memory 写法
		dsn = ":memory:"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	// SQLite 单连接更可控；MVP 单玩家场景不需要并发写。
	db.SetMaxOpenConns(1)

	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode = WAL;"); err != nil {
		// in-memory 库无法切 WAL，忽略错误
		_ = err
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply migrations: %w", err)
	}
	return &Store{db: db}, nil
}

// Close 关闭底层连接。
func (s *Store) Close() error { return s.db.Close() }

// Repo 返回一个绑定到主连接的 Repository（非事务）。
func (s *Store) Repo() *Repository { return &Repository{q: dbQuerier{s.db}} }

// RunTurn 在事务内运行 fn。fn 返回 error → 整事务回滚；否则提交。
//
// 用于 SLA 校验失败时回滚整回合写入。
func (s *Store) RunTurn(ctx context.Context, fn func(ctx context.Context, r *Repository) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	r := &Repository{q: txQuerier{tx}}
	if err := fn(ctx, r); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// querier 抽象 *sql.DB 与 *sql.Tx 的共同子集，让 Repository 在两种语境下复用同一组方法。
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type dbQuerier struct{ db *sql.DB }

func (q dbQuerier) ExecContext(ctx context.Context, sqlStr string, args ...any) (sql.Result, error) {
	return q.db.ExecContext(ctx, sqlStr, args...)
}
func (q dbQuerier) QueryContext(ctx context.Context, sqlStr string, args ...any) (*sql.Rows, error) {
	return q.db.QueryContext(ctx, sqlStr, args...)
}
func (q dbQuerier) QueryRowContext(ctx context.Context, sqlStr string, args ...any) *sql.Row {
	return q.db.QueryRowContext(ctx, sqlStr, args...)
}

type txQuerier struct{ tx *sql.Tx }

func (q txQuerier) ExecContext(ctx context.Context, sqlStr string, args ...any) (sql.Result, error) {
	return q.tx.ExecContext(ctx, sqlStr, args...)
}
func (q txQuerier) QueryContext(ctx context.Context, sqlStr string, args ...any) (*sql.Rows, error) {
	return q.tx.QueryContext(ctx, sqlStr, args...)
}
func (q txQuerier) QueryRowContext(ctx context.Context, sqlStr string, args ...any) *sql.Row {
	return q.tx.QueryRowContext(ctx, sqlStr, args...)
}
