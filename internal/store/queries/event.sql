-- name: AppendEvent :execlastid
INSERT INTO events (
    save_id,
    turn,
    type,
    description,
    related_entities_json,
    created_at
) VALUES (?, ?, ?, ?, ?, ?);

-- name: ListEvents :many
SELECT
    id,
    save_id,
    turn,
    type,
    description,
    related_entities_json,
    created_at
FROM events
WHERE save_id = ?
  AND (? = 0 OR turn >= ?)
  AND (? = 0 OR turn <= ?)
ORDER BY turn ASC, id ASC;
