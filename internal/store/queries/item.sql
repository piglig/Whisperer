-- name: UpsertItem :exec
INSERT INTO items (
    id,
    save_id,
    name,
    description,
    owner_type,
    owner_id,
    properties_json,
    destroyed
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    save_id = excluded.save_id,
    name = excluded.name,
    description = excluded.description,
    owner_type = excluded.owner_type,
    owner_id = excluded.owner_id,
    properties_json = excluded.properties_json,
    destroyed = excluded.destroyed;

-- name: GetItem :one
SELECT
    id,
    save_id,
    name,
    description,
    owner_type,
    owner_id,
    properties_json,
    destroyed
FROM items
WHERE id = ?;

-- name: MoveItem :execrows
UPDATE items
SET owner_type = ?, owner_id = ?
WHERE id = ?;

-- name: DestroyItem :execrows
UPDATE items
SET destroyed = 1
WHERE id = ?;

-- name: ListDestroyedItems :many
SELECT
    id,
    save_id,
    name,
    description,
    owner_type,
    owner_id,
    properties_json,
    destroyed
FROM items
WHERE save_id = ? AND destroyed = 1
ORDER BY name;
