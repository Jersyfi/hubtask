-- The settings catalogue's one storage shape (migration 0208): a workspace's, a hub's, a
-- collection's and a person's entries, read for the places a resolution walks through.

-- name: ListSettingValues :many
-- Every entry under a key prefix at the given places, `levels[i]` pairing with `scopes[i]`.
SELECT sv.level, sv.scope_id, sv.key, sv.value, sv.hold, sv.allowed, sv.set_by, sv.updated_at
FROM setting_value sv
WHERE (sv.level, sv.scope_id) IN (
    SELECT unnest(sqlc.arg('levels')::text[]), unnest(sqlc.arg('scopes')::uuid[])
  )
  AND sv.key LIKE sqlc.arg('prefix')::text || '%'
ORDER BY sv.key;

-- name: UpsertSettingValue :exec
INSERT INTO setting_value (id, tenant_id, level, scope_id, key, value, hold, allowed, set_by, updated_at)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('level'), sqlc.arg('scope_id'), sqlc.arg('key'),
  sqlc.narg('value'), sqlc.arg('hold'), sqlc.narg('allowed'), sqlc.narg('set_by'), sqlc.arg('updated_at')
)
ON CONFLICT (tenant_id, level, scope_id, key) DO UPDATE
  SET value = excluded.value, hold = excluded.hold, allowed = excluded.allowed,
      set_by = excluded.set_by, updated_at = excluded.updated_at;

-- name: DeleteSettingValue :execrows
DELETE FROM setting_value
WHERE level = sqlc.arg('level') AND scope_id = sqlc.arg('scope_id') AND key = sqlc.arg('key');

-- name: InstanceSettingsUnder :many
-- The installation's entries under a key prefix: instance_setting has no row level security, and
-- every workspace reads the installation's level.
SELECT key, value, lock_origin FROM instance_setting
WHERE key LIKE sqlc.arg('prefix')::text || '%'
ORDER BY key;

-- name: PlanSettingsOfWorkspaceUnder :many
-- The transaction's own workspace's plan's entries under a key prefix, empty where it is on none.
SELECT ps.key, ps.value, ps.locked
FROM tenant t
JOIN plan_setting ps ON ps.plan_id = t.plan_id
WHERE t.id = current_tenant_id() AND t.deleted_at IS NULL AND ps.key LIKE sqlc.arg('prefix')::text || '%'
ORDER BY ps.key;
