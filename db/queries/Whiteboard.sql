-- The whiteboard module's boards (migration 0240, M4). The tenant is the transaction's in every
-- statement (ADR-0010).

-- name: InsertWhiteboard :exec
INSERT INTO whiteboard (id, tenant_id, hub_id, title, scene, created_by, created_at, updated_by, updated_at, version,
  parent_page_id, order_key)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('hub_id'), sqlc.arg('title'), sqlc.arg('scene'),
  sqlc.arg('created_by'), sqlc.arg('at'), sqlc.arg('created_by'), sqlc.arg('at'), 1,
  sqlc.narg('parent_page_id'), sqlc.narg('order_key'));

-- name: FindWhiteboard :one
SELECT id, hub_id, title, scene, deleted_at, created_by, created_at, updated_by, updated_at, version,
       parent_page_id, order_key
FROM whiteboard
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: FindWhiteboardForUpdate :one
SELECT id, hub_id, title, scene, deleted_at, created_by, created_at, updated_by, updated_at, version,
       parent_page_id, order_key
FROM whiteboard
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')
FOR UPDATE;

-- name: UpdateWhiteboard :execrows
-- Optimistic: zero rows is a board changed meanwhile.
UPDATE whiteboard SET
  title = sqlc.arg('title'), scene = sqlc.arg('scene'), deleted_at = sqlc.narg('deleted_at'),
  updated_by = sqlc.arg('updated_by'), updated_at = sqlc.arg('updated_at'), version = version + 1,
  parent_page_id = sqlc.narg('parent_page_id'), order_key = sqlc.narg('order_key'),
  -- Who trashed it is written by the write that trashes it - its writer - and kept while it stays
  -- in the trash; a board no longer trashed names nobody.
  deleted_by_type = CASE
    WHEN sqlc.narg('deleted_at')::timestamptz IS NULL THEN NULL
    WHEN deleted_at IS NULL THEN NULLIF(sqlc.narg('deleted_by_type')::text, '')
    ELSE deleted_by_type END,
  deleted_by_id = CASE
    WHEN sqlc.narg('deleted_at')::timestamptz IS NULL THEN NULL
    WHEN deleted_at IS NULL THEN sqlc.arg('updated_by')
    ELSE deleted_by_id END
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id') AND version = sqlc.arg('expected_version');

-- name: ListHubWhiteboards :many
-- A hub's boards, the last changed first, without their scenes; the trash or the rest; after the
-- board `after` names when it is given (a page's cursor).
SELECT w.id, w.hub_id, w.title, jsonb_array_length(w.scene)::int AS elements, w.deleted_at,
       w.created_by, w.created_at, w.updated_by, w.updated_at, w.version, w.parent_page_id, w.order_key
FROM whiteboard w
WHERE w.tenant_id = current_tenant_id() AND w.hub_id = sqlc.arg('hub_id')
  AND (w.deleted_at IS NOT NULL) = sqlc.arg('trashed')::boolean
  AND (sqlc.narg('after')::uuid IS NULL OR (w.updated_at, w.id) < (
    SELECT c.updated_at, c.id FROM whiteboard c
    WHERE c.tenant_id = current_tenant_id() AND c.id = sqlc.narg('after')::uuid))
ORDER BY w.updated_at DESC, w.id DESC
LIMIT sqlc.arg('batch');

-- name: ListHubWhiteboardsTree :many
-- A hub's boards outside the trash in the content tree's order (UC-CNV-10 check 1): by parent, the
-- top first, then by key; a board a pod of the previous release wrote without a key after the
-- keyed ones, by title.
SELECT w.id, w.hub_id, w.title, jsonb_array_length(w.scene)::int AS elements, w.deleted_at,
       w.created_by, w.created_at, w.updated_by, w.updated_at, w.version, w.parent_page_id, w.order_key
FROM whiteboard w
WHERE w.tenant_id = current_tenant_id() AND w.hub_id = sqlc.arg('hub_id') AND w.deleted_at IS NULL
ORDER BY w.parent_page_id NULLS FIRST, w.order_key NULLS LAST, lower(w.title), w.id
LIMIT sqlc.arg('batch');

-- name: LockHubWhiteboards :exec
-- Taken before a hub's boards are counted against their ceiling, for the rest of the transaction:
-- two creates at once count in turn, so neither passes the ceiling on the other's stale count.
SELECT pg_advisory_xact_lock(hashtextextended('whiteboards/' || sqlc.arg('hub_id')::uuid::text, 0));

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
ORDER BY w.updated_at DESC, w.id DESC
LIMIT sqlc.arg('batch');

-- name: WhiteboardsByID :many
-- The boards a reference names, for resolving it; trashed ones are not found.
SELECT id, hub_id, title, jsonb_array_length(scene)::int AS elements
FROM whiteboard
WHERE tenant_id = current_tenant_id() AND id = ANY(sqlc.arg('ids')::uuid[]) AND deleted_at IS NULL;

-- name: LastWhiteboardVersion :one
SELECT seq, title, scene, author_id, name, created_at FROM whiteboard_version
WHERE tenant_id = current_tenant_id() AND whiteboard_id = sqlc.arg('whiteboard_id')
ORDER BY seq DESC LIMIT 1;

-- name: InsertWhiteboardVersion :exec
INSERT INTO whiteboard_version (tenant_id, whiteboard_id, seq, title, scene, author_id, name, created_at)
VALUES (current_tenant_id(), sqlc.arg('whiteboard_id'), sqlc.arg('seq'), sqlc.arg('title'), sqlc.arg('scene'),
  sqlc.narg('author_id'), '', sqlc.arg('at'));

-- name: FoldWhiteboardVersion :exec
UPDATE whiteboard_version SET title = sqlc.arg('title'), scene = sqlc.arg('scene'),
  author_id = sqlc.narg('author_id'), created_at = sqlc.arg('at')
WHERE tenant_id = current_tenant_id() AND whiteboard_id = sqlc.arg('whiteboard_id') AND seq = sqlc.arg('seq');

-- name: NameWhiteboardVersion :execrows
UPDATE whiteboard_version SET name = sqlc.arg('name')
WHERE tenant_id = current_tenant_id() AND whiteboard_id = sqlc.arg('whiteboard_id') AND seq = sqlc.arg('seq');

-- name: ListWhiteboardVersions :many
-- A board's history newest first, without the scenes.
SELECT seq, title, author_id, name, created_at, jsonb_array_length(scene)::int AS elements FROM whiteboard_version
WHERE tenant_id = current_tenant_id() AND whiteboard_id = sqlc.arg('whiteboard_id')
  AND (sqlc.arg('before')::int = 0 OR seq < sqlc.arg('before')::int)
ORDER BY seq DESC LIMIT sqlc.arg('batch');

-- name: FindWhiteboardVersion :one
SELECT seq, title, scene, author_id, name, created_at FROM whiteboard_version
WHERE tenant_id = current_tenant_id() AND whiteboard_id = sqlc.arg('whiteboard_id') AND seq = sqlc.arg('seq');

-- name: AttachWhiteboardMedia :exec
-- A media object placed on a board; placing it twice is once.
INSERT INTO whiteboard_media (tenant_id, whiteboard_id, media_id)
VALUES (current_tenant_id(), sqlc.arg('whiteboard_id'), sqlc.arg('media_id'))
ON CONFLICT DO NOTHING;

-- name: WhiteboardHoldsMedia :one
SELECT EXISTS (
  SELECT 1 FROM whiteboard_media
  WHERE tenant_id = current_tenant_id() AND whiteboard_id = sqlc.arg('whiteboard_id') AND media_id = sqlc.arg('media_id')
) AS held;

-- name: CountWhiteboardMedia :one
SELECT count(*)::int FROM whiteboard_media
WHERE tenant_id = current_tenant_id() AND whiteboard_id = sqlc.arg('whiteboard_id');

-- name: InsertWhiteboardComment :exec
INSERT INTO whiteboard_comment (id, tenant_id, whiteboard_id, parent_id, author_id, body, element_id, x, y, created_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('whiteboard_id'), sqlc.narg('parent_id'), sqlc.arg('author_id'),
  sqlc.arg('body'), sqlc.arg('element_id'), sqlc.arg('x'), sqlc.arg('y'), sqlc.arg('at'), 1);

-- name: FindWhiteboardComment :one
SELECT id, whiteboard_id, parent_id, author_id, body, element_id, x, y, resolved_at, resolved_by,
       created_at, edited_at, deleted_at, version
FROM whiteboard_comment
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: ListWhiteboardComments :many
SELECT id, whiteboard_id, parent_id, author_id, body, element_id, x, y, resolved_at, resolved_by,
       created_at, edited_at, deleted_at, version
FROM whiteboard_comment
WHERE tenant_id = current_tenant_id() AND whiteboard_id = sqlc.arg('whiteboard_id')
ORDER BY created_at, id
LIMIT sqlc.arg('batch');

-- name: UpdateWhiteboardComment :execrows
UPDATE whiteboard_comment SET body = sqlc.arg('body'), resolved_at = sqlc.narg('resolved_at'),
  resolved_by = sqlc.narg('resolved_by'), edited_at = sqlc.narg('edited_at'),
  deleted_at = sqlc.narg('deleted_at'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id') AND version = sqlc.arg('expected_version');

-- name: WhiteboardParentPage :one
-- The hub of a page a board is placed under, outside the trash (UC-CNV-10).
SELECT hub_id FROM page
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id') AND deleted_at IS NULL;

-- name: ContentSiblingPages :many
-- The pages directly under a parent of a hub's content tree (none: its top), outside the trash.
SELECT id, order_key FROM page
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id') AND deleted_at IS NULL
  AND parent_id IS NOT DISTINCT FROM sqlc.narg('parent_id')::uuid
ORDER BY order_key, id;

-- name: ContentSiblingWhiteboards :many
-- The boards directly under a parent of a hub's content tree (none: its top) that have a key,
-- outside the trash.
SELECT id, order_key::text AS order_key FROM whiteboard
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id') AND deleted_at IS NULL
  AND order_key IS NOT NULL AND parent_page_id IS NOT DISTINCT FROM sqlc.narg('parent_id')::uuid
ORDER BY order_key, id;

-- name: WhiteboardsUnderPages :many
-- The boards outside the trash standing directly under any of these pages, in tree order - what a
-- page's trash lifts (UC-CNV-10 check 4).
SELECT id FROM whiteboard
WHERE tenant_id = current_tenant_id() AND deleted_at IS NULL
  AND parent_page_id = ANY(sqlc.arg('page_ids')::uuid[])
ORDER BY order_key NULLS LAST, lower(title), id;
