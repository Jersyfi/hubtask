-- The whiteboard module's boards (migration 0233, M4). The tenant is the transaction's in every
-- statement (ADR-0010).

-- name: InsertWhiteboard :exec
INSERT INTO whiteboard (id, tenant_id, hub_id, title, scene, created_by, created_at, updated_by, updated_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('hub_id'), sqlc.arg('title'), sqlc.arg('scene'),
  sqlc.arg('created_by'), sqlc.arg('at'), sqlc.arg('created_by'), sqlc.arg('at'), 1);

-- name: FindWhiteboard :one
SELECT id, hub_id, title, scene, deleted_at, created_by, created_at, updated_by, updated_at, version
FROM whiteboard
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: FindWhiteboardForUpdate :one
SELECT id, hub_id, title, scene, deleted_at, created_by, created_at, updated_by, updated_at, version
FROM whiteboard
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')
FOR UPDATE;

-- name: UpdateWhiteboard :execrows
-- Optimistic: zero rows is a board changed meanwhile.
UPDATE whiteboard SET
  title = sqlc.arg('title'), scene = sqlc.arg('scene'), deleted_at = sqlc.narg('deleted_at'),
  updated_by = sqlc.arg('updated_by'), updated_at = sqlc.arg('updated_at'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id') AND version = sqlc.arg('expected_version');

-- name: ListHubWhiteboards :many
-- A hub's boards, the last changed first, without their scenes; the trash or the rest.
SELECT id, hub_id, title, jsonb_array_length(scene)::int AS elements, deleted_at,
       created_by, created_at, updated_by, updated_at, version
FROM whiteboard
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id')
  AND (deleted_at IS NOT NULL) = sqlc.arg('trashed')::boolean
ORDER BY updated_at DESC, id
LIMIT sqlc.arg('batch');

-- name: CountHubWhiteboards :one
SELECT count(*)::int FROM whiteboard
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id') AND deleted_at IS NULL;

-- name: WhiteboardsReferencing :many
-- The boards that point at an object, the strongest drawing of each; a trashed board points
-- nowhere (the trigger keeps no row for it).
SELECT w.id, w.hub_id, w.title, r.strength
FROM whiteboard_reference r JOIN whiteboard w ON w.tenant_id = r.tenant_id AND w.id = r.whiteboard_id
WHERE r.tenant_id = current_tenant_id() AND r.target_kind = sqlc.arg('target_kind')
  AND r.target_id = sqlc.arg('target_id')
ORDER BY w.updated_at DESC, w.id
LIMIT sqlc.arg('batch');

-- name: WhiteboardsByID :many
-- The boards a reference names, for resolving it; trashed ones are not found.
SELECT id, hub_id, title, jsonb_array_length(scene)::int AS elements
FROM whiteboard
WHERE tenant_id = current_tenant_id() AND id = ANY(sqlc.arg('ids')::uuid[]) AND deleted_at IS NULL;
