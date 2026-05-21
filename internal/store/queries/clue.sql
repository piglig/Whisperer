-- name: UpsertClue :exec
INSERT INTO clues (
    id,
    save_id,
    scenario_id,
    description,
    found,
    found_in_location_id,
    found_at_turn
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    save_id = excluded.save_id,
    scenario_id = excluded.scenario_id,
    description = excluded.description,
    found = excluded.found,
    found_in_location_id = excluded.found_in_location_id,
    found_at_turn = excluded.found_at_turn;

-- name: MarkClueFound :execrows
UPDATE clues
SET found = 1, found_in_location_id = ?, found_at_turn = ?
WHERE id = ?;

-- name: ListFoundClues :many
SELECT
    id,
    save_id,
    scenario_id,
    description,
    found,
    found_in_location_id,
    found_at_turn
FROM clues
WHERE save_id = ? AND found = 1
ORDER BY found_at_turn;
