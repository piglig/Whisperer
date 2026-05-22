-- name: CreateSave :exec
INSERT INTO saves (
    id,
    name,
    scenario_id,
    variant_id,
    current_location_id,
    turn_count,
    time_of_day,
    created_at,
    updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetSave :one
SELECT *
FROM saves
WHERE id = ?;

-- name: ListSaves :many
SELECT *
FROM saves
ORDER BY updated_at DESC;

-- name: SetTimeOfDay :execrows
UPDATE saves
SET time_of_day = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteSave :execrows
DELETE FROM saves
WHERE id = ?;

-- name: UpdateSaveProgress :execrows
UPDATE saves
SET current_location_id = ?, turn_count = ?, updated_at = ?
WHERE id = ?;
