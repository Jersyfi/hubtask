-- The tenant is never a parameter here: it comes from the transaction's own context through
-- current_tenant_id(), which is the same value row level security compares against. A row can
-- therefore not be written into the wrong tenant even by a caller that wanted to, and a read
-- cannot reach one belonging to somebody else.

-- name: FindContainer :one
-- Trashed and archived containers are returned rather than filtered out. The repository reports
-- what is stored and judges none of it: whether a container may take children is a question the
-- domain answers, and a query that hid a trashed parent would turn "it is in the trash" into
-- "it does not exist" (I-C2, I-C3).
--
-- One key of `policies` is read out, not the column: the completion policy has a reader (B-07) and the
-- other three keys do not, and selecting a value nothing consumes is a promise nothing keeps. `->>`
-- yields NULL for a collection that has never been configured, and coalesce turns that into the empty
-- string - which the domain reads as the default. Coalescing here rather than mapping a nil pointer in
-- the adapter keeps the generated field a plain string, and "unset" one concept instead of two.
SELECT
  id, tenant_id, type, parent_id, name, description, icon, color_token, order_key,
  coalesce(policies->>'completion_policy', '')::text AS completion_policy,
  archived_at, deleted_at, trash_batch_id, created_by, created_at, updated_at, version
FROM container
WHERE id = $1;

-- name: LastContainerOrderKey :one
-- The highest rank directly under a parent, or no row when the level is empty. A NULL parent is
-- the hub level, which is why the comparison is IS NOT DISTINCT FROM rather than `=`: `NULL = NULL`
-- is unknown, and the hubs would come back as no rows at all.
--
-- Trashed containers count. Their rank is still occupied - a restore has to land where it was.
SELECT order_key
FROM container
WHERE parent_id IS NOT DISTINCT FROM sqlc.narg('parent_id')::uuid
ORDER BY order_key DESC
LIMIT 1;

-- name: InsertContainer :exec
-- The name is stored Unicode NFC normalised, in the database rather than in the application:
-- "Übersicht" typed with a combining diaeresis and the same word composed are one name to a
-- person, and the unique index has to see them as one name too. Doing it here also keeps the
-- domain free of a Unicode library it is not allowed to import (ADR-0001).
INSERT INTO container (
  id, tenant_id, type, parent_id, name, description, icon, color_token, order_key,
  created_by, created_at, updated_at, version
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('type'), sqlc.narg('parent_id'),
  normalize(sqlc.arg('name')::text, NFC),
  sqlc.narg('description'), sqlc.narg('icon'), sqlc.narg('color_token'), sqlc.arg('order_key'),
  sqlc.arg('created_by'), sqlc.arg('created_at'), sqlc.arg('created_at'), 1
);

-- name: FindWorkItem :one
-- Trashed and archived items are returned rather than filtered out, for the reason FindContainer
-- returns them: the repository reports what is stored and judges none of it (I-W4).
SELECT
  id, tenant_id, collection_id, type, parent_id, path, depth, title, notes,
  is_completed, completed_at, completed_by, order_key,
  archived_at, deleted_at, trash_batch_id, created_by, created_at, updated_at, version
FROM work_item
WHERE id = $1;

-- name: LastWorkItemOrderKey :one
-- The highest rank among the siblings of a new item: same collection, same parent. A NULL parent
-- is the level directly under the collection, which is why the comparison is IS NOT DISTINCT FROM
-- rather than `=` - `NULL = NULL` is unknown, and the tasks would come back as no rows at all.
--
-- Trashed items count. Their rank is still occupied; a restore has to land where it was.
SELECT order_key
FROM work_item
WHERE collection_id = sqlc.arg('collection_id')::uuid
  AND parent_id IS NOT DISTINCT FROM sqlc.narg('parent_id')::uuid
ORDER BY order_key DESC
LIMIT 1;

-- name: InsertWorkItem :exec
-- The title is stored Unicode NFC normalised, in the database rather than in the application, for
-- the reason the container's name is: two spellings of the same word have to be one value to
-- every query that compares or searches them, and doing it here keeps a Unicode library out of
-- the domain (ADR-0001, I-W7).
--
-- The fields this use case does not own are absent rather than defaulted: bucket, labels,
-- members, assignee, due date, cover, custom fields and the recurrence rule are written by the
-- use cases that own them, and their columns carry NULL until then.
INSERT INTO work_item (
  id, tenant_id, collection_id, type, parent_id, path, depth, title, notes,
  order_key, created_by, created_at, updated_at, version
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('collection_id'), sqlc.arg('type'),
  sqlc.narg('parent_id'), sqlc.arg('path'), sqlc.arg('depth'),
  normalize(sqlc.arg('title')::text, NFC),
  sqlc.narg('notes'), sqlc.arg('order_key'), sqlc.arg('created_by'),
  sqlc.arg('created_at'), sqlc.arg('created_at'), 1
);
