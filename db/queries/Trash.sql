-- The trash and the archive: the statements that move rows between the two lifecycle stamps, and
-- the ones that read and finally remove what is in the trash (B-10).
--
-- Their own file rather than more of Work.sql, because they are one subject with one invariant
-- running through all of them - the batch identifier every row of one deletion shares (I-C2) - and
-- because the file that holds the live tree's reads should not be where somebody looks for a delete.
--
-- The tenant is never a parameter here either: it comes from the transaction's own context through
-- current_tenant_id(), which is the value row level security compares against.

-- name: SetWorkItemArchived :execrows
-- The archive stamp on one item, set or cleared. One statement for both directions, because they
-- differ in the value and in nothing else.
--
-- Only this row. Archiving is a decision about one entry and does not descend: a work package under
-- an archived task stays writable, unlike a collection under an archived hub, because an item's
-- children are entries in their own right rather than a level of the structure (I-C3 is about
-- containers). Optimistic locking in the WHERE clause, as everywhere.
UPDATE work_item SET
  archived_at = sqlc.narg('archived_at'),
  updated_at  = sqlc.arg('updated_at'),
  version     = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: SetWorkItemTrashed :execrows
-- The deletion stamp on the row the caller actually read, set or cleared, with the batch it belongs
-- to. Both columns move together: a stamp without a batch could not be restored as part of anything,
-- and a batch without a stamp is a row that is not in the trash claiming to be part of a deletion.
--
-- This row alone, and it is excluded from the two statements below for the reason
-- SetWorkItemPlacement is excluded from MoveWorkItemSubtree: one deletion moves one version on the
-- row the optimistic lock is on, and writing it twice would be an artefact of splitting the work in
-- two rather than a design.
UPDATE work_item SET
  deleted_at     = sqlc.narg('deleted_at'),
  trash_batch_id = sqlc.narg('trash_batch_id'),
  updated_at     = sqlc.arg('updated_at'),
  version        = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: TrashWorkItemDescendants :execrows
-- Everything below one item goes into the trash with it, under the same batch.
--
-- `LIKE prefix || '%'` rather than starts_with, because that is the form wi_path_idx
-- (tenant_id, path text_pattern_ops) serves as an index scan. A path is built from UUIDs and
-- separators, so there is nothing in the data for a `%` or a `_` to do.
--
-- `deleted_at IS NULL` is the whole of invariant I-C2's second half: a descendant already in the
-- trash from an earlier, separate deletion keeps that deletion. Adopting it into this batch would
-- restart its retention period and make a restore of this batch bring back something nobody deleted
-- this time.
UPDATE work_item SET
  deleted_at     = sqlc.arg('deleted_at'),
  trash_batch_id = sqlc.arg('trash_batch_id')::uuid,
  updated_at     = sqlc.arg('updated_at'),
  version        = version + 1
WHERE path LIKE sqlc.arg('prefix')::text || '%'
  AND id <> sqlc.arg('item_id')::uuid
  AND deleted_at IS NULL;

-- name: RestoreWorkItemBatch :execrows
-- The way back: every item of one deletion, taken out of the trash together.
--
-- Keyed on the batch and not on a path, which is what makes a restore exactly reverse the deletion it
-- undoes. A subtree that has grown a younger, separate deletion inside it since would otherwise come
-- back whole, and a subtree that has been moved apart in the meantime would not come back at all.
--
-- It serves a container's deletion as well as an item's: the items of a trashed collection carry the
-- container batch, so this is the statement that gives them back too.
UPDATE work_item SET
  deleted_at     = NULL,
  trash_batch_id = NULL,
  updated_at     = sqlc.arg('updated_at'),
  version        = version + 1
WHERE trash_batch_id = sqlc.arg('trash_batch_id')::uuid
  AND id <> sqlc.arg('item_id')::uuid;

-- name: SetContainerTrashed :execrows
-- The deletion stamp on the hub or collection the caller read, set or cleared, with its batch.
UPDATE container SET
  deleted_at     = sqlc.narg('deleted_at'),
  trash_batch_id = sqlc.narg('trash_batch_id'),
  updated_at     = sqlc.arg('updated_at'),
  version        = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: TrashCollectionsOfHub :many
-- A hub's collections go into the trash with it, under the same batch (I-C2).
--
-- It returns the identifiers rather than a count, because they are needed twice afterwards: the items
-- of those collections are the next statement's argument, and each collection is announced separately
-- to offline clients - a device that subscribes to one collection rather than to the hub would
-- otherwise never learn that its collection is gone (offline-sync.md §3.1).
--
-- `deleted_at IS NULL` for the reason it is on the item statement: a collection already in the trash
-- keeps its own deletion, and is therefore not in this batch and not in this answer.
UPDATE container SET
  deleted_at     = sqlc.arg('deleted_at'),
  trash_batch_id = sqlc.arg('trash_batch_id')::uuid,
  updated_at     = sqlc.arg('updated_at'),
  version        = version + 1
WHERE parent_id = sqlc.arg('hub_id')::uuid
  AND deleted_at IS NULL
RETURNING id;

-- name: TrashItemsOfCollections :execrows
-- Every item of the collections a container's deletion covers, under the same batch.
--
-- The whole of each collection in one statement, at every level: an item's collection is denormalised
-- onto every row of its subtree (domain-model.md §3.4), so there is no walk to do here - which is
-- what keeps deleting a hub a fixed number of statements rather than one per level.
UPDATE work_item SET
  deleted_at     = sqlc.arg('deleted_at'),
  trash_batch_id = sqlc.arg('trash_batch_id')::uuid,
  updated_at     = sqlc.arg('updated_at'),
  version        = version + 1
WHERE collection_id = ANY(sqlc.arg('collection_ids')::uuid[])
  AND deleted_at IS NULL;

-- name: RestoreContainerBatch :many
-- The containers of one deletion, taken out of the trash together. Returns what came back, for the
-- same reason the trashing statement does: each one is announced separately.
UPDATE container SET
  deleted_at     = NULL,
  trash_batch_id = NULL,
  updated_at     = sqlc.arg('updated_at'),
  version        = version + 1
WHERE trash_batch_id = sqlc.arg('trash_batch_id')::uuid
  AND id <> sqlc.arg('container_id')::uuid
RETURNING id;

-- name: ListTrash :many
-- What is in the trash, newest deletion first.
--
-- One row per deletion rather than one per deleted row: a hub with two hundred entries under it went
-- into the trash as one act and comes back as one act, so listing its subtree would be two hundred
-- lines describing one decision. The row this returns is the batch's root - the thing a person
-- deleted - and the batch identifier beside it is what restores the rest.
--
-- The root is found by asking each row whether its parent is in the same batch. It is not, exactly
-- when the row is where the deletion started: a collection whose hub was deleted with it carries the
-- hub's batch, an item whose collection or parent item was deleted with it carries theirs, and both
-- are therefore dropped. `IS DISTINCT FROM` rather than `<>`, so that a row whose parent is not in
-- the trash at all - a NULL on the joined side - counts as a root rather than as unknown.
--
-- `hub_id` is the level the permission question is asked at. A membership held at a hub applies
-- downwards (domain-model.md §3.2), so an entry that named only its collection could not be shown to
-- somebody whose right sits on the hub above it - and the trash is the one view that spans hubs, so
-- there is no path parameter to read it from. For a hub it is null: the hub is its own level.
--
-- Both branches read through the partial trash indices, and the joins are primary key lookups.
--
-- The keyset is (deleted_at, id) descending, which is the order the page is in. A UNION of two
-- tables cannot be walked by an index alone, so the sort is real - bounded by the size of the trash,
-- which is bounded in turn by the retention period that empties it.
SELECT kind, id, trash_batch_id, deleted_at, title, subtype, hub_id, collection_id, parent_id, version
FROM (
  SELECT
    'CONTAINER'::text AS kind,
    c.id              AS id,
    c.trash_batch_id  AS trash_batch_id,
    c.deleted_at      AS deleted_at,
    c.name            AS title,
    c.type::text      AS subtype,
    c.parent_id       AS hub_id,
    NULL::uuid        AS collection_id,
    c.parent_id       AS parent_id,
    c.version         AS version
  FROM container c
  LEFT JOIN container cp ON cp.id = c.parent_id
  WHERE c.deleted_at IS NOT NULL
    AND cp.trash_batch_id IS DISTINCT FROM c.trash_batch_id

  UNION ALL

  SELECT
    'ITEM'::text, i.id, i.trash_batch_id, i.deleted_at, i.title, i.type::text,
    col.parent_id, i.collection_id, i.parent_id, i.version
  FROM work_item i
  JOIN container col ON col.id = i.collection_id
  LEFT JOIN work_item ip ON ip.id = i.parent_id
  WHERE i.deleted_at IS NOT NULL
    AND col.trash_batch_id IS DISTINCT FROM i.trash_batch_id
    AND ip.trash_batch_id IS DISTINCT FROM i.trash_batch_id
) entries
WHERE (
  sqlc.narg('cursor_deleted_at')::timestamptz IS NULL
  OR (deleted_at, id) < (sqlc.narg('cursor_deleted_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
)
ORDER BY deleted_at DESC, id DESC
LIMIT sqlc.arg('page_size');

-- name: WorkItemSubtreeIDs :many
-- Every identifier in one item's subtree, the item included, oldest path first.
--
-- Read before a hard delete rather than derived from it: each row that goes needs a tombstone and a
-- journal entry of its own (offline-sync.md §7, data-retention.md §5), and a cascade that removed
-- them silently would leave a device free to recreate a purged entry on its next push.
SELECT id
FROM work_item
WHERE path LIKE sqlc.arg('prefix')::text || '%'
ORDER BY path;

-- name: PurgeWorkItems :execrows
-- The hard delete, by identifier.
--
-- By identifier rather than by predicate, because the identifiers have already been read, written to
-- the journal and given tombstones - deleting a different set here than the one that was recorded is
-- exactly the orphan the completeness rule forbids (ADR-0020 §6).
DELETE FROM work_item
WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: PurgeContainers :execrows
-- The hard delete for hubs and collections.
--
-- Called for the collections before the hubs that hold them: `container.parent_id` is
-- ON DELETE RESTRICT, so a hub whose collections are still there refuses to go - which is the
-- database insisting on the order rather than a rule this code could forget.
DELETE FROM container
WHERE id = ANY(sqlc.arg('ids')::uuid[]);
