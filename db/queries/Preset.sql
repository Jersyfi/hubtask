-- Saved presets (UC-WRK-30): the workspace's and each person's own.
--
-- The tenant is never a parameter: it comes from the transaction's context through
-- current_tenant_id(), the value row level security compares against. Which person may read which
-- own preset is the application's decision (rule 2); the queries only narrow by what they are given.

-- name: InsertStructurePreset :exec
INSERT INTO structure_preset (
  tenant_id, id, origin, owner_id, name, settings, created_at, version
) VALUES (
  current_tenant_id(), sqlc.arg('id'), sqlc.arg('origin'), sqlc.narg('owner_id'),
  normalize(sqlc.arg('name')::text, NFC), sqlc.arg('settings')::jsonb, sqlc.arg('created_at'), 1
);

-- name: FindStructurePreset :one
SELECT id, origin, owner_id, name, settings, created_at, version
FROM structure_preset
WHERE id = sqlc.arg('id')::uuid;

-- name: ListStructurePresetsOffered :many
-- The workspace's presets and one person's own, by name; `id` closes the order.
SELECT id, origin, owner_id, name, settings, created_at, version
FROM structure_preset
WHERE origin = 'WORKSPACE' OR owner_id = sqlc.arg('owner_id')::uuid
ORDER BY origin DESC, name COLLATE hubtask_name, id;

-- name: CountStructurePresets :one
-- How many presets of one origin there are - the workspace's, or one person's own.
SELECT count(*)::int
FROM structure_preset
WHERE origin = sqlc.arg('origin')
  AND (sqlc.arg('origin') = 'WORKSPACE' OR owner_id = sqlc.narg('owner_id')::uuid);

-- name: UpdateStructurePreset :execrows
UPDATE structure_preset SET
  name       = normalize(sqlc.arg('name')::text, NFC),
  settings   = sqlc.arg('settings')::jsonb,
  version    = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: DeleteStructurePreset :execrows
DELETE FROM structure_preset
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');
