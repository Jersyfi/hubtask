-- The tenant is never a parameter here: it comes from the transaction's own context through
-- current_tenant_id(), which is the same value row level security compares against. A row can
-- therefore not be written into the wrong tenant even by a caller that wanted to, and a read
-- cannot reach one belonging to somebody else.

-- name: FindContainer :one
-- Trashed and archived containers are returned rather than filtered out. The repository reports
-- what is stored and judges none of it: whether a container may take children is a question the
-- domain answers, and a query that hid a trashed parent would turn "it is in the trash" into
-- "it does not exist" (I-C2, I-C3).
SELECT
  id, tenant_id, type, parent_id, name, description, icon, color_token, order_key,
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

-- name: ListContainers :many
-- One level of the container tree, in its manual order: the hubs when no parent is named, that
-- hub's collections when one is. IS NOT DISTINCT FROM is what makes the absent parent mean the hub
-- level rather than "no filter" - `parent_id = NULL` is unknown, and the hubs would come back as no
-- rows at all.
--
-- The type filter composes with it rather than replacing it, which means the two impossible
-- combinations return an empty page: a collection always has a parent, and a hub never does. That is
-- the filters agreeing, not a special case worth coding.
--
-- Trashed rows are never here - the trash is its own view (B-10). Archived ones are, when the caller
-- asks: an archived collection is still a collection, and hiding it would make it unreachable.
--
-- The keyset is (order_key, id) rather than an offset, so a page boundary survives a concurrent
-- insert (api-guidelines.md §4). `id` is the tiebreak the guidelines require, and it is what makes
-- the boundary unambiguous should two siblings ever share a rank.
--
-- COLLATE "C" on both the comparison and the order, for the reason migration 0007 gives: a rank key
-- is a fractional index whose scheme rests on byte order, and a database created en_US.utf8 on glibc
-- would order it differently from the domain that produced it.
--
-- One row more than the page size is read, and the caller reports has_more from it: asking whether
-- there is a next page is otherwise a second query, and a COUNT over the level is the expensive
-- thing this endpoint exists to avoid.
SELECT
  id, tenant_id, type, parent_id, name, description, icon, color_token, order_key,
  archived_at, deleted_at, trash_batch_id, created_by, created_at, updated_at, version
FROM container
WHERE parent_id IS NOT DISTINCT FROM sqlc.narg('parent_id')::uuid
  AND deleted_at IS NULL
  AND (sqlc.narg('type')::container_type IS NULL OR type = sqlc.narg('type')::container_type)
  AND (sqlc.arg('include_archived')::boolean OR archived_at IS NULL)
  AND (
    sqlc.narg('cursor_order_key')::text IS NULL
    OR (order_key COLLATE "C", id)
       > (sqlc.narg('cursor_order_key')::text COLLATE "C", sqlc.narg('cursor_id')::uuid)
  )
ORDER BY order_key COLLATE "C", id
LIMIT sqlc.arg('page_size');

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

-- name: ListWorkItems :many
-- One level of one collection, in its manual order: the items directly in the collection when no
-- parent is named, that item's children when one is. Anchored to a collection because an unanchored
-- list of every item in a tenant is an unindexed scan, and every filter beyond one level is what
-- POST /items:query exists for (B-12).
--
-- The collection is compared as well as the parent, even when the parent decides the level on its
-- own: it is the leading column of wi_level_order_idx, and a query that dropped it would fall back to
-- wi_parent_idx and scan the whole tenant's tasks for the parent-IS-NULL case.
--
-- Everything else - the keyset, the collation, the row read beyond the page - is as ListContainers,
-- and for the same reasons.
SELECT
  id, tenant_id, collection_id, type, parent_id, path, depth, title, notes,
  is_completed, completed_at, completed_by, order_key,
  archived_at, deleted_at, trash_batch_id, created_by, created_at, updated_at, version
FROM work_item
WHERE collection_id = sqlc.arg('collection_id')::uuid
  AND parent_id IS NOT DISTINCT FROM sqlc.narg('parent_id')::uuid
  AND deleted_at IS NULL
  AND (sqlc.arg('include_archived')::boolean OR archived_at IS NULL)
  AND (
    sqlc.narg('cursor_order_key')::text IS NULL
    OR (order_key COLLATE "C", id)
       > (sqlc.narg('cursor_order_key')::text COLLATE "C", sqlc.narg('cursor_id')::uuid)
  )
ORDER BY order_key COLLATE "C", id
LIMIT sqlc.arg('page_size');
