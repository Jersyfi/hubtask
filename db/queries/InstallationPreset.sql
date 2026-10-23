-- The installation's presets (UC-INS-18). No tenant predicate: the table carries no policy, and
-- every workspace reads it; writing it is the control plane's, in a transaction without a tenant.

-- name: ListInstallationPresets :many
-- Every preset by name; `id` closes the order.
SELECT id, name, settings, is_default, locks, version
FROM installation_preset
ORDER BY name COLLATE hubtask_name, id;

-- name: FindInstallationPreset :one
SELECT id, name, settings, is_default, locks, version
FROM installation_preset
WHERE id = sqlc.arg('id')::uuid;

-- name: DefaultInstallationPreset :one
-- The default, where there is one: what a hub starts from and what the installation locks.
SELECT id, name, settings, is_default, locks, version
FROM installation_preset
WHERE is_default;

-- name: CountInstallationPresets :one
SELECT count(*)::int FROM installation_preset;

-- name: InsertInstallationPreset :execrows
-- A new preset. A name taken is no row written, which the adapter answers as `presets.name_taken`.
INSERT INTO installation_preset (
  id, name, settings, is_default, locks, version, created_at, updated_at, updated_by
) VALUES (
  sqlc.arg('id'), normalize(sqlc.arg('name')::text, NFC), sqlc.arg('settings')::jsonb,
  sqlc.arg('is_default'), sqlc.arg('locks')::text[], 1, sqlc.arg('now'), sqlc.arg('now'),
  sqlc.narg('updated_by')
)
ON CONFLICT (name) DO NOTHING;

-- name: UpdateInstallationPreset :execrows
-- Writes a preset whole under the expected version; none expected writes whatever stands - the
-- instance file, which is the source.
UPDATE installation_preset SET
  name       = normalize(sqlc.arg('name')::text, NFC),
  settings   = sqlc.arg('settings')::jsonb,
  is_default = sqlc.arg('is_default'),
  locks      = sqlc.arg('locks')::text[],
  version    = version + 1,
  updated_at = sqlc.arg('now'),
  updated_by = sqlc.narg('updated_by')
WHERE id = sqlc.arg('id')::uuid
  AND (sqlc.narg('expected_version')::int IS NULL OR version = sqlc.narg('expected_version')::int);

-- name: ClearInstallationDefault :exec
-- The former default stops being one and loses its locks, before another becomes the default.
UPDATE installation_preset SET
  is_default = false,
  locks      = '{}',
  version    = version + 1,
  updated_at = sqlc.arg('now'),
  updated_by = sqlc.narg('updated_by')
WHERE is_default AND id <> sqlc.arg('except_id')::uuid;

-- name: DeleteInstallationPreset :execrows
DELETE FROM installation_preset
WHERE id = sqlc.arg('id')::uuid
  AND (sqlc.narg('expected_version')::int IS NULL OR version = sqlc.narg('expected_version')::int);
