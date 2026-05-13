-- +goose Up
-- +goose StatementBegin
PRAGMA foreign_keys = ON;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS saves (
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
CREATE TABLE IF NOT EXISTS investigators (
    id TEXT PRIMARY KEY,
    save_id TEXT NOT NULL REFERENCES saves(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    occupation TEXT NOT NULL,
    attrs_json TEXT NOT NULL,
    skills_json TEXT NOT NULL,
    hp INTEGER NOT NULL,
    mp INTEGER NOT NULL,
    san INTEGER NOT NULL,
    inventory_json TEXT NOT NULL DEFAULT '[]',
    active INTEGER NOT NULL DEFAULT 1
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS npcs (
    id TEXT PRIMARY KEY,
    save_id TEXT NOT NULL REFERENCES saves(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    personality TEXT NOT NULL,
    knowledge_json TEXT NOT NULL DEFAULT '{}',
    relation_to_player INTEGER NOT NULL DEFAULT 0,
    location_id TEXT,
    alive INTEGER NOT NULL DEFAULT 1
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS locations (
    id TEXT PRIMARY KEY,
    save_id TEXT NOT NULL REFERENCES saves(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    parent_id TEXT,
    connections_json TEXT NOT NULL DEFAULT '[]',
    visited INTEGER NOT NULL DEFAULT 0
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS items (
    id TEXT PRIMARY KEY,
    save_id TEXT NOT NULL REFERENCES saves(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    owner_type TEXT NOT NULL CHECK (owner_type IN ('npc', 'location', 'investigator', 'none')),
    owner_id TEXT,
    properties_json TEXT NOT NULL DEFAULT '{}',
    destroyed INTEGER NOT NULL DEFAULT 0
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS clues (
    id TEXT PRIMARY KEY,
    save_id TEXT NOT NULL REFERENCES saves(id) ON DELETE CASCADE,
    scenario_id TEXT NOT NULL,
    description TEXT NOT NULL,
    found INTEGER NOT NULL DEFAULT 0,
    found_in_location_id TEXT,
    found_at_turn INTEGER
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    save_id TEXT NOT NULL REFERENCES saves(id) ON DELETE CASCADE,
    turn INTEGER NOT NULL,
    type TEXT NOT NULL,
    description TEXT NOT NULL,
    related_entities_json TEXT NOT NULL DEFAULT '[]',
    created_at INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_events_save_turn ON events(save_id, turn);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_npcs_save_location ON npcs(save_id, location_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_items_save_owner ON items(save_id, owner_type, owner_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_investigators_save_active ON investigators(save_id, active);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_clues_save_found ON clues(save_id, found);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS clues;
DROP TABLE IF EXISTS items;
DROP TABLE IF EXISTS locations;
DROP TABLE IF EXISTS npcs;
DROP TABLE IF EXISTS investigators;
DROP TABLE IF EXISTS saves;
-- +goose StatementEnd
