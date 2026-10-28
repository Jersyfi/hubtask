-- The type registry (migration 0209, UC-FLW-01). A workspace's rows only: the system types a
-- workspace has never changed are the domain's and have none.

-- name: ListEntryTypes :many
SELECT id, tenant_id, key, level, system, name, icon, color_token, completable, workflow, record,
  schedulable, estimable, allowed_children, position, archived_at, created_at, updated_at, version
FROM entry_type
WHERE tenant_id = current_tenant_id()
ORDER BY level, position, key;

-- name: FindEntryType :one
SELECT id, tenant_id, key, level, system, name, icon, color_token, completable, workflow, record,
  schedulable, estimable, allowed_children, position, archived_at, created_at, updated_at, version
FROM entry_type
WHERE tenant_id = current_tenant_id() AND key = sqlc.arg('key')::text;

-- name: InsertEntryType :exec
INSERT INTO entry_type (
  id, tenant_id, key, level, system, name, icon, color_token, completable, workflow, record,
  schedulable, estimable, allowed_children, position, archived_at, created_at, updated_at, version
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('key'), sqlc.arg('level'), sqlc.arg('system'),
  normalize(sqlc.arg('name')::text, NFC), sqlc.arg('icon'), sqlc.arg('color_token'),
  sqlc.arg('completable'), sqlc.arg('workflow'), sqlc.arg('record'), sqlc.arg('schedulable'),
  sqlc.arg('estimable'), sqlc.arg('allowed_children')::text[], sqlc.arg('position'),
  sqlc.narg('archived_at'), sqlc.arg('created_at'), sqlc.arg('created_at'), 1
);

-- name: UpdateEntryType :execrows
-- The whole row as the application decided it, under the optimistic lock (api-guidelines.md §5).
-- The key, the level and whether it is a system type never change.
UPDATE entry_type SET
  name             = normalize(sqlc.arg('name')::text, NFC),
  icon             = sqlc.arg('icon'),
  color_token      = sqlc.arg('color_token'),
  completable      = sqlc.arg('completable'),
  workflow         = sqlc.arg('workflow'),
  record           = sqlc.arg('record'),
  schedulable      = sqlc.arg('schedulable'),
  estimable        = sqlc.arg('estimable'),
  allowed_children = sqlc.arg('allowed_children')::text[],
  position         = sqlc.arg('position'),
  archived_at      = sqlc.narg('archived_at'),
  updated_at       = sqlc.arg('updated_at'),
  version          = version + 1
WHERE tenant_id = current_tenant_id() AND key = sqlc.arg('key')::text
  AND version = sqlc.arg('expected_version');

-- name: DeleteEntryType :execrows
-- Only a type no entry carries, trashed ones included; the reference refuses the others too.
DELETE FROM entry_type
WHERE tenant_id = current_tenant_id() AND key = sqlc.arg('key')::text AND NOT system
  AND NOT EXISTS (
    SELECT 1 FROM work_item wi
    WHERE wi.tenant_id = current_tenant_id() AND wi.entry_type = sqlc.arg('key')::text
  );

-- name: EntryTypeInUse :one
SELECT EXISTS (
  SELECT 1 FROM work_item wi
  WHERE wi.tenant_id = current_tenant_id() AND wi.entry_type = sqlc.arg('key')::text
)::boolean AS in_use;

-- name: SetWorkItemEntryType :execrows
-- The entry's type; null is its level's system type. Under the optimistic lock every write to an
-- entry takes (api-guidelines.md §5).
UPDATE work_item SET
  entry_type = sqlc.narg('entry_type'),
  updated_at = sqlc.arg('updated_at'),
  version    = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');
