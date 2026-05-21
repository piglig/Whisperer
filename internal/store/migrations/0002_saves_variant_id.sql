-- +goose Up
-- 给 saves 表加 variant_id 列。每局开始随机选一个 variant 后写入此列；reload 时按
-- 此列重 merge effective scenario，保证读档与开档一致。
--
-- Fresh goose-managed databases apply this after 0001_init.

-- +goose StatementBegin
ALTER TABLE saves ADD COLUMN variant_id TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- SQLite 不直接支持 DROP COLUMN（在 3.35 之前），用 table-recreate 模式：
-- +goose StatementBegin
CREATE TABLE saves_new (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    scenario_id TEXT NOT NULL,
    current_location_id TEXT,
    turn_count INTEGER NOT NULL DEFAULT 0,
    time_of_day TEXT NOT NULL DEFAULT 'morning' CHECK (time_of_day IN ('morning','afternoon','night')),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
-- +goose StatementEnd
-- +goose StatementBegin
INSERT INTO saves_new (id, name, scenario_id, current_location_id, turn_count, time_of_day, created_at, updated_at)
SELECT id, name, scenario_id, current_location_id, turn_count, time_of_day, created_at, updated_at FROM saves;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE saves;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE saves_new RENAME TO saves;
-- +goose StatementEnd
