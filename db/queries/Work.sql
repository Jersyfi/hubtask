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
--
-- The hub's own archive stamp travels with the row as `parent_archived_at`, which is invariant I-C3's
-- second half: a collection in an archived hub is read-only without being archived itself. Read here
-- rather than stored on the collection, so that archiving a hub writes one row and unarchiving it
-- restores exactly what it covered - a collection archived in its own right stays archived. The join
-- is a primary key lookup, and it is NULL for a hub, which sits under nothing.
SELECT
  c.id, c.tenant_id, c.type, c.parent_id, c.name, c.description, c.icon, c.color_token, c.order_key,
  coalesce(c.policies->>'completion_policy', '')::text AS completion_policy,
  c.archived_at, parent.archived_at AS parent_archived_at,
  c.deleted_at, c.trash_batch_id, c.created_by, c.created_at, c.updated_at, c.version
FROM container c
LEFT JOIN container parent ON parent.id = c.parent_id
WHERE c.id = $1;

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
-- asks: an archived collection is still a collection, and hiding it would make it unreachable. The
-- filter reads the row's own stamp rather than the effective one on purpose - a collection is not
-- hidden from the level because the hub above it was archived, since the client asking for that hub's
-- collections has just been told the hub is archived.
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
  c.id, c.tenant_id, c.type, c.parent_id, c.name, c.description, c.icon, c.color_token, c.order_key,
  coalesce(c.policies->>'completion_policy', '')::text AS completion_policy,
  c.archived_at, parent.archived_at AS parent_archived_at,
  c.deleted_at, c.trash_batch_id, c.created_by, c.created_at, c.updated_at, c.version
FROM container c
LEFT JOIN container parent ON parent.id = c.parent_id
WHERE c.parent_id IS NOT DISTINCT FROM sqlc.narg('parent_id')::uuid
  AND c.deleted_at IS NULL
  AND (sqlc.narg('type')::container_type IS NULL OR c.type = sqlc.narg('type')::container_type)
  AND (sqlc.arg('include_archived')::boolean OR c.archived_at IS NULL)
  AND (
    sqlc.narg('cursor_order_key')::text IS NULL
    OR (c.order_key COLLATE "C", c.id)
       > (sqlc.narg('cursor_order_key')::text COLLATE "C", sqlc.narg('cursor_id')::uuid)
  )
ORDER BY c.order_key COLLATE "C", c.id
LIMIT sqlc.arg('page_size');

-- name: SetContainerAttributes :execrows
-- A container's own descriptive fields: what RenameContainer may change (B-06).
--
-- Every column is written on every call, not only the ones that moved. The application has already
-- decided what the row should say - it read the container, applied the update and refused what the
-- lifecycle does not allow - so this writes that decision whole. `description = COALESCE($1,
-- description)` would additionally make clearing a description unexpressible.
--
-- The name is normalised here for the reason InsertContainer normalises it: the unique index has to
-- see two spellings of the same word as one name, and the domain may not import a Unicode library
-- (ADR-0001). A rename into a name already taken at this level therefore fails on `container_name_uq`
-- exactly as an insert does, which is what keeps one answer for one condition.
--
-- Optimistic locking in the WHERE clause, as everywhere: the update matches nothing when somebody
-- else has moved the row on, and the caller learns that rather than overwriting them
-- (api-guidelines.md §5).
UPDATE container SET
  name        = normalize(sqlc.arg('name')::text, NFC),
  description = sqlc.narg('description'),
  icon        = sqlc.narg('icon'),
  color_token = sqlc.narg('color_token'),
  updated_at  = sqlc.arg('updated_at'),
  version     = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: SetContainerPolicies :execrows
-- One key of the policies document, set in place.
--
-- `jsonb_set` rather than replacing the column, because the column holds four documented keys and this
-- use case owns one of them. Overwriting the whole document would silently discard a default bucket or
-- a capability override the moment either of those arrives - a data loss nobody would see until the
-- feature that wrote them stopped working.
--
-- The value is a parameter of the jsonb constructor rather than concatenated text: `to_jsonb` on a
-- bound parameter is what keeps this a parameterised statement, which rule 9 requires of every query
-- including the ones that build a document.
UPDATE container SET
  policies   = jsonb_set(policies, '{completion_policy}',
               to_jsonb(sqlc.arg('completion_policy')::text), true),
  updated_at = sqlc.arg('updated_at'),
  version    = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: SetContainerArchived :execrows
-- The archive stamp, set or cleared. One statement for both directions, because they differ in the
-- value and in nothing else.
--
-- Only this row. A collection under an archived hub inherits the state through FindContainer's join
-- rather than through a stamp of its own, so archiving a hub writes one row here however many
-- collections sit in it - and unarchiving it restores exactly what it covered, leaving a collection
-- that was archived in its own right archived (I-C3).
UPDATE container SET
  archived_at = sqlc.narg('archived_at'),
  updated_at  = sqlc.arg('updated_at'),
  version     = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: SetContainerPlacement :execrows
-- Where a collection sits and how it ranks there: the whole of what a move changes.
--
-- No subtree statement beside it, unlike the item move. A container tree is two levels deep (I-C1), so
-- a collection has no containers below it whose path would have to follow - and the items it holds
-- reference their collection rather than a path through the hubs.
--
-- A name already taken in the destination fails on `container_name_uq`, which is the same index that
-- decides an insert. Checking beforehand would be two statements with a gap, and two moves arriving
-- inside that gap would both pass the check (multi-tenancy.md §2.1).
UPDATE container SET
  parent_id  = sqlc.arg('parent_id')::uuid,
  order_key  = sqlc.arg('order_key'),
  updated_at = sqlc.arg('updated_at'),
  version    = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: ContainerOrderKeyNeighbours :one
-- The two ranks a position sits between at one container level: the rank of the container to go
-- before, and the greatest rank below it.
--
-- The counterpart of OrderKeyNeighbours for items, and the same shape for the same reasons: both
-- bounds in one row so that they are consistent as of the moment they are asked, the empty string for
-- "nothing there", and COLLATE "C" wherever two ranks are compared - a fractional index rests on byte
-- order, and a database created en_US.utf8 on glibc would order it differently from the domain that
-- produced it.
--
-- The level is the parent, compared with IS NOT DISTINCT FROM so that an absent one means the hubs
-- rather than no filter at all. The moving container is excluded from its own level: a reorder would
-- otherwise measure a position against the rank it is leaving.
WITH level AS (
  SELECT id, order_key
  FROM container
  WHERE parent_id IS NOT DISTINCT FROM sqlc.narg('parent_id')::uuid
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

-- name: FindWorkItem :one
-- Trashed and archived items are returned rather than filtered out, for the reason FindContainer
-- returns them: the repository reports what is stored and judges none of it (I-W4).
SELECT
  id, tenant_id, collection_id, type, parent_id, path, depth, title, notes,
  is_completed, completed_at, completed_by, bucket_id, order_key, assignee_id,
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
-- The fields this use case does not own are absent rather than defaulted: labels, members,
-- assignee, due date, cover, custom fields and the recurrence rule are written by the use cases
-- that own them, and their columns carry NULL until then.
INSERT INTO work_item (
  id, tenant_id, collection_id, type, parent_id, path, depth, title, notes,
  bucket_id, order_key, created_by, created_at, updated_at, version
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('collection_id'), sqlc.arg('type'),
  sqlc.narg('parent_id'), sqlc.arg('path'), sqlc.arg('depth'),
  normalize(sqlc.arg('title')::text, NFC),
  sqlc.narg('notes'), sqlc.narg('bucket_id'), sqlc.arg('order_key'), sqlc.arg('created_by'),
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
  is_completed, completed_at, completed_by, bucket_id, order_key, assignee_id,
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
-- name: ChildCompletion :one
-- How many children an item has, and how many of them are done. The two numbers the roll-up decides
-- from (I-W5), as counts rather than as rows: the question is "is anything still open down there", and
-- reading a subtree to answer one boolean is the shape this avoids.
--
-- Trashed children are excluded outright. They are deletions waiting out their retention period, and a
-- work package whose last activity was deleted must not become done because of it. Archived children are
-- counted as they stand - archiving is a decision to keep something quietly, not to disown it.
--
-- Served by wi_parent_idx (tenant_id, parent_id, order_key): the tenant comes from row level security, so
-- the leading column is satisfied without appearing here.
SELECT
  count(*)::int AS total,
  (count(*) FILTER (WHERE is_completed))::int AS completed
FROM work_item
WHERE parent_id = sqlc.arg('parent_id')::uuid
  AND deleted_at IS NULL;

-- name: SetWorkItemCompletion :execrows
-- Optimistic locking in the WHERE clause: the update matches nothing when somebody else has moved the row
-- on, and the caller learns that rather than overwriting them (api-guidelines.md §5).
--
-- Both completion columns are written together, always. The table's own CHECK insists that
-- `is_completed = (completed_at IS NOT NULL)`, so a statement that set one without the other would be
-- refused by the database - which is the right place for that rule and the reason this query has no
-- branch in it.
UPDATE work_item SET
  is_completed = sqlc.arg('is_completed'),
  completed_at = sqlc.narg('completed_at'),
  completed_by = sqlc.narg('completed_by'),
  updated_at   = sqlc.arg('updated_at'),
  version      = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');
-- name: SetWorkItemAssignee :execrows
-- The one person an entry is on, set or cleared, under the same optimistic lock every write to
-- this row takes: the update matches nothing when somebody else has moved the row on, and the
-- caller learns that rather than overwriting them (api-guidelines.md §5).
--
-- Its own statement rather than a column added to SetWorkItemAttributes. An assignment is one
-- decision about one field, and a statement that wrote the title alongside it would make handing
-- an entry to somebody spend the version of a rename nobody asked for - which is the same reason
-- the completion has a statement of its own.
--
-- Whether that account may be assigned at all is not asked here. It is a question about a
-- membership somewhere above the entry, decided in the application layer before this runs
-- (ADR-0005); the tenant-scoped foreign key is what stops the identifier pointing outside the
-- tenant, and it is the database's rather than this statement's (ADR-0024).
UPDATE work_item SET
  assignee_id = sqlc.narg('assignee_id'),
  updated_at  = sqlc.arg('updated_at'),
  version     = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: SetWorkItemAttributes :execrows
-- The item's own fields: what UpdateWorkItem may change in 0.2.0 (B-05).
--
-- Both columns are written on every call, not only the ones that moved. The application has already
-- decided what the row should say - it read the item, applied the update and refused what the capability
-- profile does not allow - so this writes that decision whole. A statement that switched on which fields
-- were sent would be the second place deciding it, in the layer that is not allowed to decide anything
-- (ADR-0005), and `notes = COALESCE($1, notes)` would additionally make clearing the notes unexpressible.
--
-- Optimistic locking in the WHERE clause, as everywhere: the update matches nothing when somebody else has
-- moved the row on, and the caller learns that rather than overwriting them (api-guidelines.md §5).
--
-- `search_vector` follows by itself. It is a generated column over title and notes, so the index behind
-- full text search cannot fall behind a rename - which is exactly what a trigger somebody has to remember
-- would eventually do.
UPDATE work_item SET
  title      = sqlc.arg('title'),
  notes      = sqlc.narg('notes'),
  bucket_id  = sqlc.narg('bucket_id'),
  updated_at = sqlc.arg('updated_at'),
  version    = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

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
-- The version moves on every row, because a descendant's path is part of its state and a client caching the
-- subtree has to be able to tell that it changed.
--
-- The moved item itself is excluded. Its own path begins with its own prefix, so it would match - and it is
-- SetWorkItemPlacement that owns its row, which is where the optimistic lock is. Without the exclusion the
-- moved item would have its version bumped twice by one move, which is not a design but an artefact of
-- writing it in two statements.
UPDATE work_item SET
  collection_id = sqlc.arg('collection_id')::uuid,
  path          = sqlc.arg('new_prefix')::text
                  || substring(path from length(sqlc.arg('old_prefix')::text) + 1),
  depth         = depth + sqlc.arg('depth_delta')::int,
  updated_at    = sqlc.arg('updated_at'),
  version       = version + 1
WHERE path LIKE sqlc.arg('old_prefix')::text || '%'
  AND id <> sqlc.arg('item_id')::uuid;

-- name: SetWorkItemPlacement :execrows
-- The moved item's own row: the parent it now sits under and the rank it takes among its new siblings.
--
-- This row is written here in full and excluded from the subtree statement, so that one move moves one
-- version. Only this row's parent changes - a descendant keeps the parent it had - and the optimistic lock
-- belongs on the row the caller actually read, which is this one.
UPDATE work_item SET
  parent_id     = sqlc.narg('parent_id')::uuid,
  collection_id = sqlc.arg('collection_id')::uuid,
  path          = sqlc.arg('path'),
  depth         = sqlc.arg('depth'),
  order_key     = sqlc.arg('order_key'),
  bucket_id     = sqlc.narg('bucket_id')::uuid,
  updated_at    = sqlc.arg('updated_at'),
  version       = version + 1
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
