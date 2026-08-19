-- The structure of a collection: the buckets its items are arranged in, and the labels they are
-- tagged with (B-09).
--
-- The tenant is never a parameter here, exactly as in Work.sql: it comes from the transaction's own
-- context through current_tenant_id(), which is the same value row level security compares against.
-- A row can therefore not be written into the wrong tenant even by a caller that wanted to.

-- name: FindBucket :one
-- Deleted buckets are returned rather than filtered out. What the state means is the domain's
-- decision; a query that hid a deleted bucket would turn "it has been deleted" into "it never
-- existed", and a client repeating a delete would get the wrong answer to the wrong question.
SELECT
  id, tenant_id, collection_id, name, order_key, wip_limit, is_done_bucket, color_token,
  deleted_at, version
FROM bucket
WHERE id = $1;

-- name: ListBuckets :many
-- A collection's board, left to right. Not paged: a board has as many columns as fit on a screen,
-- and the contract returns a plain array rather than a page (api-guidelines.md §2). A collection
-- that grows thousands of them has a different problem than pagination solves.
--
-- Deleted buckets are never here. Unlike Find, this answers "what is on the board", where a deleted
-- column is not - the same distinction ListWorkItems keeps against FindWorkItem.
--
-- COLLATE "C" for the reason migration 0007 gives: a rank key is a fractional index whose scheme
-- rests on byte order, and a database created en_US.utf8 on glibc would order it differently from
-- the domain that produced it. Stated in the query rather than served by an index of its own - a
-- board is a handful of rows, and a sort over it is cheaper than a second index to keep.
--
-- `id` closes the order so that two columns sharing a rank still come back in one stable sequence.
SELECT
  id, tenant_id, collection_id, name, order_key, wip_limit, is_done_bucket, color_token,
  deleted_at, version
FROM bucket
WHERE collection_id = $1 AND deleted_at IS NULL
ORDER BY order_key COLLATE "C", id;

-- name: LastBucketOrderKey :one
-- The highest rank among a collection's buckets, or no row when it has none.
--
-- Deleted buckets count. Their rank is still occupied - a restore has to land where it was, and
-- reusing the key would put two columns in the same place.
SELECT order_key
FROM bucket
WHERE collection_id = $1
ORDER BY order_key COLLATE "C" DESC
LIMIT 1;

-- name: InsertBucket :exec
-- The name is stored Unicode NFC normalised, in the database rather than in the application, for
-- the reason InsertContainer normalises it: "Prüfung" typed with a combining diaeresis and the same
-- word composed are one name to a person, and the unique index has to see them as one name too.
-- Doing it here also keeps the domain free of a Unicode library it may not import (ADR-0001).
INSERT INTO bucket (
  id, tenant_id, collection_id, name, order_key, wip_limit, is_done_bucket, color_token, version
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('collection_id'),
  normalize(sqlc.arg('name')::text, NFC), sqlc.arg('order_key'),
  sqlc.narg('wip_limit'), sqlc.arg('is_done_bucket'), sqlc.narg('color_token'), 1
);

-- name: SetBucketAttributes :execrows
-- Every column is written on every call, not only the ones that moved: the application has already
-- decided what the row should say, so this writes that decision whole. `wip_limit =
-- COALESCE($1, wip_limit)` would additionally make clearing a limit unexpressible.
--
-- Optimistic locking in the WHERE clause, as everywhere: the update matches nothing when somebody
-- else has moved the row on, and the caller learns that rather than overwriting them
-- (api-guidelines.md §5).
UPDATE bucket SET
  name           = normalize(sqlc.arg('name')::text, NFC),
  wip_limit      = sqlc.narg('wip_limit'),
  is_done_bucket = sqlc.arg('is_done_bucket'),
  color_token    = sqlc.narg('color_token'),
  version        = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: SetBucketOrderKey :execrows
-- The whole of what a reorder changes. A board is one level, so nothing below a column has to
-- follow it.
UPDATE bucket SET
  order_key = sqlc.arg('order_key'),
  version   = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: SetBucketDeleted :execrows
-- A soft delete: the row stays so that a change log entry can still name it and a restore stays
-- possible (offline-sync.md §7). The unique name index is partial on `deleted_at IS NULL`, so the
-- name is free again the moment this runs.
UPDATE bucket SET
  deleted_at = sqlc.arg('deleted_at'),
  version    = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: BucketOrderKeyNeighbours :one
-- The two ranks a position sits between on one board: the rank of the bucket to go before, and the
-- greatest rank below it.
--
-- The counterpart of ContainerOrderKeyNeighbours, and the same shape for the same reasons: both
-- bounds in one row so that they are consistent as of the moment they are asked, the empty string
-- for "nothing there", and COLLATE "C" wherever two ranks are compared.
--
-- The moving bucket is excluded from its own board: a reorder would otherwise measure a position
-- against the rank it is leaving.
WITH level AS (
  SELECT id, order_key
  FROM bucket
  WHERE collection_id = sqlc.arg('collection_id')::uuid
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

-- name: FirstBucket :one
-- The collection's leftmost live bucket, ignoring one: the bucket a deleted column's items fall
-- back to (B-09).
--
-- Derived rather than stored. `default_bucket_id` is a documented key of the policies document and
-- no use case writes it, so a stored default would be a value nothing keeps up to date - a column
-- deleted while the key still named it would send items into a bucket that is no longer on the
-- board. The leftmost column cannot drift out of step with the columns that exist.
SELECT
  id, tenant_id, collection_id, name, order_key, wip_limit, is_done_bucket, color_token,
  deleted_at, version
FROM bucket
WHERE collection_id = sqlc.arg('collection_id')::uuid
  AND deleted_at IS NULL
  AND id <> sqlc.arg('excluded_id')::uuid
ORDER BY order_key COLLATE "C", id
LIMIT 1;

-- name: MoveItemsBetweenBuckets :execrows
-- Every item in one bucket into another, or out of any bucket when the target is NULL. What
-- DeleteBucket owes its items: the alternative is the foreign key's ON DELETE SET NULL, which would
-- take a person's board apart silently.
--
-- Trashed items are moved too. Their bucket is part of the state a restore brings back, and leaving
-- them pointing at a deleted column would restore them onto a board that no longer has it.
UPDATE work_item SET
  bucket_id  = sqlc.narg('target_bucket_id'),
  updated_at = sqlc.arg('updated_at'),
  version    = version + 1
WHERE bucket_id = sqlc.arg('source_bucket_id')::uuid;

-- name: FindLabel :one
-- Deleted labels are returned rather than filtered out, for the reason FindBucket returns deleted
-- buckets: what the state means is the domain's decision.
SELECT
  id, tenant_id, collection_id, name, color_token, description, deleted_at, version
FROM label
WHERE id = $1;

-- name: ListLabels :many
-- A collection's vocabulary, in the order a person reads it: by name. Not by a rank, because a
-- label has none - it is a chip in a set rather than a column in a sequence, and inventing an order
-- for it would be a field nothing means.
--
-- The order is the database's collation deliberately, unlike the boards: this one is read by people
-- and "Ä" belongs beside "A" for them, which is exactly what a byte order would not do.
SELECT
  id, tenant_id, collection_id, name, color_token, description, deleted_at, version
FROM label
WHERE collection_id = $1 AND deleted_at IS NULL
ORDER BY name, id;

-- name: InsertLabel :exec
INSERT INTO label (
  id, tenant_id, collection_id, name, color_token, description, version
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('collection_id'),
  normalize(sqlc.arg('name')::text, NFC), sqlc.arg('color_token'), sqlc.narg('description'), 1
);

-- name: SetLabelAttributes :execrows
UPDATE label SET
  name        = normalize(sqlc.arg('name')::text, NFC),
  color_token = sqlc.arg('color_token'),
  description = sqlc.narg('description'),
  version     = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: SetLabelDeleted :execrows
-- A soft delete. The rows in item_label stay: an item that carried the label goes on carrying it,
-- and the read side filters on the label's own stamp - which is what makes a deletion undoable
-- without having to remember who wore it.
UPDATE label SET
  deleted_at = sqlc.arg('deleted_at'),
  version    = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: ListItemLabels :many
-- The labels one item carries, deleted ones left out: the label is gone from the collection's
-- vocabulary, so it is gone from the chips a client renders, without anything having had to rewrite
-- the item.
SELECT l.id
FROM item_label il
JOIN label l ON l.id = il.label_id
WHERE il.item_id = $1 AND l.deleted_at IS NULL
ORDER BY l.name, l.id;

-- name: AddItemLabel :exec
-- ON CONFLICT DO NOTHING rather than a check first: adding a label an item already carries is the
-- state the caller asked for, and two requests arriving together would otherwise both pass a check
-- and one of them fail on the primary key.
INSERT INTO item_label (tenant_id, item_id, label_id)
VALUES (current_tenant_id(), sqlc.arg('item_id'), sqlc.arg('label_id'))
ON CONFLICT DO NOTHING;

-- name: RemoveItemLabel :execrows
DELETE FROM item_label
WHERE item_id = sqlc.arg('item_id')::uuid AND label_id = sqlc.arg('label_id')::uuid;

-- name: RecordSetElementAdded :exec
-- The OR-set tag of an addition (offline-sync.md §4.2, §10).
--
-- The add tag is kept and the remove tag cleared, so that "added later than it was removed" is a
-- comparison a merge can make rather than a history it has to reconstruct. Without these tags a
-- label added on one device would be lost the moment another device removed a different one -
-- which is exactly what last writer wins over a whole array does.
INSERT INTO set_element (tenant_id, item_id, set_name, element_id, add_tag, remove_tag)
VALUES (current_tenant_id(), sqlc.arg('item_id'), sqlc.arg('set_name'), sqlc.arg('element_id'),
        sqlc.arg('tag'), NULL)
ON CONFLICT (tenant_id, item_id, set_name, element_id) DO UPDATE
  SET add_tag = excluded.add_tag, remove_tag = NULL;

-- name: RecordSetElementRemoved :exec
-- The OR-set tag of a removal. The add tag stays: a merge decides membership by comparing the two,
-- and a removal that erased the addition would make a concurrent re-add on another device
-- indistinguishable from never having happened.
INSERT INTO set_element (tenant_id, item_id, set_name, element_id, add_tag, remove_tag)
VALUES (current_tenant_id(), sqlc.arg('item_id'), sqlc.arg('set_name'), sqlc.arg('element_id'),
        NULL, sqlc.arg('tag'))
ON CONFLICT (tenant_id, item_id, set_name, element_id) DO UPDATE
  SET remove_tag = excluded.remove_tag;

-- name: ListSetElements :many
-- Every tag of one set on one item: what a merge compares a client's tags against
-- (offline-sync.md §4.2).
SELECT element_id, add_tag, remove_tag
FROM set_element
WHERE item_id = sqlc.arg('item_id')::uuid AND set_name = sqlc.arg('set_name')
ORDER BY element_id;

-- name: ClearForeignSubtreeLabels :many
-- Every label an entry in a moved subtree carries that its new collection does not define, removed
-- and reported (invariant I-W6).
--
-- A move to another collection takes an entry away from the vocabulary it was tagged from, and a
-- label from elsewhere would be a reference that resolves to a word nobody in the new collection
-- chose. Deleting them in one statement rather than walking the subtree keeps the transaction's
-- length independent of how many entries moved, and RETURNING is what lets the answer name what was
-- lost - I-W6 asks for the losses to be reported, never silently dropped.
--
-- `LIKE prefix || '%'` rather than starts_with, for the reason MoveWorkItemSubtree gives: that is
-- the form wi_path_idx serves as an index scan, and a path built from UUIDs and separators contains
-- no LIKE metacharacter.
DELETE FROM item_label il
USING work_item wi, label l
WHERE il.item_id = wi.id
  AND il.label_id = l.id
  AND wi.path LIKE sqlc.arg('path_prefix')::text || '%'
  AND l.collection_id <> sqlc.arg('collection_id')::uuid
RETURNING il.item_id, il.label_id;

-- name: ClearForeignSubtreeBuckets :many
-- Every entry in a moved subtree that still points at a column of the collection it left, taken off
-- that board and reported (invariant I-W6).
--
-- Only a task carries a bucket and only the moved item itself can be one, so this touches at most
-- one row today - it is written over the subtree all the same, because which types carry a board is
-- a capability profile rather than a rule this query gets to assume (ADR-0006).
--
-- The version is deliberately not moved here. This runs as part of a move, and the two statements
-- that move the subtree bump every row in it - including this one. Bumping it twice would make one
-- move look like two to a client that caches versions, and would leave the moved item's own row on
-- a version the optimistic lock in SetWorkItemPlacement no longer matches.
UPDATE work_item wi SET
  bucket_id = NULL
FROM bucket b
WHERE wi.bucket_id = b.id
  AND wi.path LIKE sqlc.arg('path_prefix')::text || '%'
  AND b.collection_id <> sqlc.arg('collection_id')::uuid
RETURNING wi.id, b.id AS bucket_id;
