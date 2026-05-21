-- name: UpsertInvestigator :exec
INSERT INTO investigators (
    id,
    save_id,
    name,
    occupation,
    attrs_json,
    skills_json,
    hp,
    mp,
    san,
    inventory_json,
    active
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    save_id = excluded.save_id,
    name = excluded.name,
    occupation = excluded.occupation,
    attrs_json = excluded.attrs_json,
    skills_json = excluded.skills_json,
    hp = excluded.hp,
    mp = excluded.mp,
    san = excluded.san,
    inventory_json = excluded.inventory_json,
    active = excluded.active;

-- name: GetActiveInvestigator :one
SELECT
    id,
    save_id,
    name,
    occupation,
    attrs_json,
    skills_json,
    hp,
    mp,
    san,
    inventory_json,
    active
FROM investigators
WHERE save_id = ? AND active = 1
LIMIT 1;

-- name: UpdateInvestigatorVitals :execrows
UPDATE investigators
SET hp = ?, mp = ?, san = ?
WHERE id = ?;

-- name: ListInvestigators :many
SELECT
    id,
    save_id,
    name,
    occupation,
    attrs_json,
    skills_json,
    hp,
    mp,
    san,
    inventory_json,
    active
FROM investigators
WHERE save_id = ?
ORDER BY name;

-- name: DeactivateInvestigator :execrows
UPDATE investigators
SET active = 0
WHERE id = ?;
