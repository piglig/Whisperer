-- name: UpsertNPC :exec
INSERT INTO npcs (
    id,
    save_id,
    name,
    personality,
    knowledge_json,
    relation_to_player,
    location_id,
    alive
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    save_id = excluded.save_id,
    name = excluded.name,
    personality = excluded.personality,
    knowledge_json = excluded.knowledge_json,
    relation_to_player = excluded.relation_to_player,
    location_id = excluded.location_id,
    alive = excluded.alive;

-- name: GetNPC :one
SELECT
    id,
    save_id,
    name,
    personality,
    knowledge_json,
    relation_to_player,
    location_id,
    alive
FROM npcs
WHERE id = ?;

-- name: ListNPCsAtLocation :many
SELECT
    id,
    save_id,
    name,
    personality,
    knowledge_json,
    relation_to_player,
    location_id,
    alive
FROM npcs
WHERE save_id = ? AND location_id = ? AND alive = 1
ORDER BY name;

-- name: UpdateNPCRelation :execrows
UPDATE npcs
SET relation_to_player = relation_to_player + ?
WHERE id = ?;

-- name: KillNPC :execrows
UPDATE npcs
SET alive = 0
WHERE id = ?;
