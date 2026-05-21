-- name: UpsertLocation :exec
INSERT INTO locations (
    id,
    save_id,
    name,
    description,
    parent_id,
    connections_json,
    visited
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    save_id = excluded.save_id,
    name = excluded.name,
    description = excluded.description,
    parent_id = excluded.parent_id,
    connections_json = excluded.connections_json,
    visited = excluded.visited;

-- name: GetLocation :one
SELECT
    id,
    save_id,
    name,
    description,
    parent_id,
    connections_json,
    visited
FROM locations
WHERE id = ?;

-- name: MarkLocationVisited :execrows
UPDATE locations
SET visited = 1
WHERE id = ?;
