-- +goose Up
-- +goose StatementBegin
ALTER TABLE saves ADD COLUMN stage TEXT NOT NULL DEFAULT 'opening';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE TABLE saves_new (
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
-- +goose StatementEnd
-- +goose StatementBegin
INSERT INTO saves_new (id, name, scenario_id, variant_id, current_location_id, turn_count, time_of_day, created_at, updated_at)
SELECT id, name, scenario_id, variant_id, current_location_id, turn_count, time_of_day, created_at, updated_at FROM saves;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE saves;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE saves_new RENAME TO saves;
-- +goose StatementEnd
