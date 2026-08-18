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

-- name: MoveWorkItemSubtree :execrows
-- Rewrites the materialised path, the depth and the collection of an item and everything below it, in one
-- statement (I-W2: the path and the depth stay consistent, and changes go through a move that updates the
-- subtree within one transaction).
--
-- The prefix swap is what makes this one statement rather than a walk: every descendant's new path is its old
-- path with the old prefix replaced, and `substring(path from length(prefix) + 1)` is that suffix. The depth
-- moves by a constant, because every row in a subtree shifts by the same number of levels.
--
-- `LIKE prefix || '%'` rather than starts_with, because that is the form wi_path_idx
-- (tenant_id, path text_pattern_ops) serves as an index scan. A path is built from UUIDs and separators, so
-- it can contain no LIKE metacharacter - there is nothing here for a `%` or a `_` in the data to do.
--
-- Trashed rows in the subtree are rewritten too, deliberately. Their path still has to describe where they
-- would be restored to; leaving them behind would point a restore at an ancestor that has moved.
--
-- The version moves on every row. A descendant's path is part of its state, and a client caching the subtree
-- has to be able to tell that it changed.
UPDATE work_item SET
  collection_id = sqlc.arg('collection_id')::uuid,
  path          = sqlc.arg('new_prefix')::text
                  || substring(path from length(sqlc.arg('old_prefix')::text) + 1),
  depth         = depth + sqlc.arg('depth_delta')::int,
  updated_at    = sqlc.arg('updated_at'),
  version       = version + 1
WHERE path LIKE sqlc.arg('old_prefix')::text || '%';

-- name: SetWorkItemPlacement :execrows
-- The moved item's own row: the parent it now sits under and the rank it takes among its new siblings.
--
-- Separate from the subtree rewrite because only this row's parent changes - a descendant keeps the parent it
-- had - and because the optimistic lock belongs on the row the caller read. The path, the depth and the
-- collection are the subtree statement's business, and this one deliberately does not touch them.
UPDATE work_item SET
  parent_id  = sqlc.narg('parent_id')::uuid,
  order_key  = sqlc.arg('order_key'),
  updated_at = sqlc.arg('updated_at'),
  version    = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: SetWorkItemOrderKey :execrows
-- A reorder within one level: the rank alone, which is the whole of what drag and drop changes.
UPDATE work_item SET
  order_key  = sqlc.arg('order_key'),
  updated_at = sqlc.arg('updated_at'),
  version    = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: OrderKeyNeighbours :one
-- The two ranks a new position sits between: the rank of the item to go before, and the greatest rank below
-- it in the same level.
--
-- Both come back as the empty string when there is nothing there, which is what the ordering service reads as
-- "no bound" - before everything, or after everything. Asking the database for both in one row rather than
-- computing one from a list keeps the answer consistent with what is stored at the moment it is asked.
--
-- COLLATE "C" on the comparison and nowhere else it could disagree: a rank key is a fractional index whose
-- scheme rests on byte order, and a database created en_US.utf8 on glibc would order it differently from the
-- domain that produced it.
--
-- The level is (collection, parent), and the parent is compared with IS NOT DISTINCT FROM so that an absent
-- one means the items directly in the collection rather than no filter at all.
WITH level AS (
  SELECT id, order_key
  FROM work_item
  WHERE collection_id = sqlc.arg('collection_id')::uuid
    AND parent_id IS NOT DISTINCT FROM sqlc.narg('parent_id')::uuid
    AND deleted_at IS NULL
    AND id <> sqlc.arg('moving_id')::uuid
), anchor AS (
  SELECT order_key FROM level WHERE id = sqlc.narg('before_id')::uuid
)
SELECT
  coalesce((SELECT order_key FROM anchor), '')::text AS next_key,
  coalesce((
    SELECT max(order_key COLLATE "C")
    FROM level
    WHERE (SELECT order_key FROM anchor) IS NULL
       OR order_key COLLATE "C" < (SELECT order_key FROM anchor) COLLATE "C"
  ), '')::text AS previous_key;
