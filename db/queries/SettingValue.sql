-- The settings catalogue's one storage shape (migration 0215): a workspace's, a hub's, a
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

-- name: ListHubsWithoutSetting :many
-- Every hub of the workspace that stores nothing under the key - archived and trashed hubs too,
-- or restoring one would change what it has.
SELECT c.id
FROM container c
WHERE c.tenant_id = current_tenant_id() AND c.type = 'HUB'
  AND NOT EXISTS (
    SELECT 1 FROM setting_value held
    WHERE held.tenant_id = current_tenant_id() AND held.level = 'HUB' AND held.scope_id = c.id
      AND held.key = sqlc.arg('key')::text
  )
ORDER BY c.id;

-- name: PinHubSettingFlags :many
-- A flag written at each hub named that stores none under the key, `ids[i]` being the new row's
-- id for `hubs[i]`, answering the hubs written. A hub that stores on under one of `on_where` gets
-- on whatever `value` says. The join keeps the write to the workspace's own hubs whatever is
-- named; the conflict target is the workspace's own row, and DO NOTHING leaves it as it is.
INSERT INTO setting_value (id, tenant_id, level, scope_id, key, value, hold, updated_at)
SELECT pin.id, current_tenant_id(), 'HUB', c.id, sqlc.arg('key')::text,
  to_jsonb(sqlc.arg('value')::boolean OR EXISTS (
    SELECT 1 FROM setting_value held
    WHERE held.tenant_id = current_tenant_id() AND held.level = 'HUB' AND held.scope_id = c.id
      AND held.key = ANY(sqlc.arg('on_where')::text[]) AND held.value = 'true'::jsonb
  )),
  'SET', sqlc.arg('updated_at')
FROM (
  SELECT unnest(sqlc.arg('ids')::uuid[]) AS id, unnest(sqlc.arg('hubs')::uuid[]) AS hub_id
) pin
JOIN container c ON c.id = pin.hub_id
WHERE c.tenant_id = current_tenant_id() AND c.type = 'HUB'
ON CONFLICT (tenant_id, level, scope_id, key) DO NOTHING
RETURNING scope_id;

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
