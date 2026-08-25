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
-- keys without a use case do not, and selecting a value nothing consumes is a promise nothing keeps.
-- `->>` yields NULL for a collection that has never been configured, and coalesce turns that into the
-- empty string - which the domain reads as the default. Coalescing here rather than mapping a nil
-- pointer in the adapter keeps the generated field a plain string, and "unset" one concept instead of
-- two.
--
-- The auto_assign key of the same document lives in its own row (C-02, see Assignment.sql), and the
-- second LEFT JOIN is how it travels with the container: NULL columns for a container without a
-- policy, which the adapter reads as the key being absent. The join lands on migration 0011's unique
-- index, so it costs an index lookup - the same price as the parent join beside it.
--
-- The hub's own archive stamp travels with the row as `parent_archived_at`, which is invariant I-C3's
-- second half: a collection in an archived hub is read-only without being archived itself. Read here
-- rather than stored on the collection, so that archiving a hub writes one row and unarchiving it
-- restores exactly what it covered - a collection archived in its own right stays archived. The join
-- is a primary key lookup, and it is NULL for a hub, which sits under nothing.
SELECT
  c.id, c.tenant_id, c.type, c.parent_id, c.name, c.description, c.icon, c.color_token, c.order_key,
  coalesce(c.policies->>'completion_policy', '')::text AS completion_policy,
  aap.strategy AS auto_assign_strategy,
  aap.candidates AS auto_assign_candidates,
  aap.enabled AS auto_assign_enabled,
  c.archived_at, parent.archived_at AS parent_archived_at,
  c.deleted_at, c.trash_batch_id, c.created_by, c.created_at, c.updated_at, c.version
FROM container c
LEFT JOIN container parent ON parent.id = c.parent_id
LEFT JOIN auto_assign_policy aap ON aap.scope_type = 'COLLECTION' AND aap.scope_id = c.id
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
  aap.strategy AS auto_assign_strategy,
  aap.candidates AS auto_assign_candidates,
  aap.enabled AS auto_assign_enabled,
  c.archived_at, parent.archived_at AS parent_archived_at,
  c.deleted_at, c.trash_batch_id, c.created_by, c.created_at, c.updated_at, c.version
FROM container c
LEFT JOIN container parent ON parent.id = c.parent_id
LEFT JOIN auto_assign_policy aap ON aap.scope_type = 'COLLECTION' AND aap.scope_id = c.id
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
  wi.id, wi.tenant_id, wi.collection_id, wi.type, wi.parent_id, wi.path, wi.depth, wi.title,
  wi.notes, wi.is_completed, wi.completed_at, wi.completed_by, wi.bucket_id, wi.order_key,
  wi.assignee_id, wi.start_at, wi.due_at, wi.due_date_only, wi.due_time_zone,
  wi.cover_kind, wi.cover_color_token, wi.cover_media_id,
  -- The visible custom fields: only the values whose own definition still lives. The hiding
  -- happens here rather than in Go, so that every read of an entry - the find, the list, the
  -- query endpoint - hides a deleted definition's values identically. Identity rather than key,
  -- because a definition recreated under the same key must not resurrect what the old one held
  -- (C-07): each value's ref names the definition it was written under (migration 0018), and a
  -- recreated key is a new definition standing behind nothing it did not write. The values
  -- themselves stay in the row untouched, which is the whole shape of the soft delete.
  (SELECT coalesce(jsonb_object_agg(kv.key, kv.value), '{}'::jsonb)
     FROM jsonb_each(wi.custom_fields) AS kv
    WHERE EXISTS (
      SELECT 1 FROM custom_field_definition cfd
       WHERE cfd.deleted_at IS NULL
         AND cfd.id = (wi.custom_field_refs ->> kv.key)::uuid
         AND (cfd.collection_id = wi.collection_id OR cfd.collection_id IS NULL)
    ))::jsonb AS custom_fields,
  wi.content_language, wi.recurrence_rule_id,
  wi.archived_at, wi.deleted_at, wi.trash_batch_id, wi.created_by, wi.created_at, wi.updated_at,
  wi.version
FROM work_item wi
WHERE wi.id = $1;

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
-- that own them, and their columns carry NULL until then. The due date a create declares reaches
-- the row through that writer in the same transaction (D-01), exactly as an assignee does; the
-- start is the create's own, a plain attribute beside the notes.
INSERT INTO work_item (
  id, tenant_id, collection_id, type, parent_id, path, depth, title, notes,
  bucket_id, order_key, start_at, content_language, created_by, created_at, updated_at, version
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('collection_id'), sqlc.arg('type'),
  sqlc.narg('parent_id'), sqlc.arg('path'), sqlc.arg('depth'),
  normalize(sqlc.arg('title')::text, NFC),
  sqlc.narg('notes'), sqlc.narg('bucket_id'), sqlc.arg('order_key'),
  sqlc.narg('start_at'), sqlc.narg('content_language'), sqlc.arg('created_by'),
  sqlc.arg('created_at'), sqlc.arg('created_at'), 1
);

-- name: SubtreeOfWorkItem :many
-- Everything below one entry, the entry itself excluded: what a copy of a subtree reads before it
-- writes anything (C-11).
--
-- The prefix match is MoveWorkItemSubtree's, and for the same reasons: every descendant's path
-- begins with the entry's own, `LIKE prefix || '%'` is the form wi_path_idx
-- (tenant_id, path text_pattern_ops) serves as an index scan, and a path built from UUIDs and
-- separators can hold no LIKE metacharacter. The entry itself matches its own prefix and is
-- excluded, because the caller already holds it and copies it under rules of its own.
--
-- Trashed rows are left out. They are on their way out of the system, and a copy that carried them
-- would put back what somebody deleted; an archived one is copied, because it is a place rather
-- than a deletion and the copy keeps it (C-11).
--
-- Ordered by depth first, so that a caller walking the rows always meets a parent before its
-- children and can carry the mapping from old identifier to new one forwards in one pass. The rank
-- decides within a level, in byte order, exactly as every other ordered read of this table
-- (ADR-0022): a rank is a fractional index and the database's collation would compare it as words.
--
-- The limit is the caller's bound rather than a page: a copy is one transaction, and a subtree
-- larger than the caller allows is refused rather than copied halfway. One row beyond the bound is
-- read on purpose, so that "too large" is distinguishable from "exactly at the bound".
SELECT
  wi.id, wi.tenant_id, wi.collection_id, wi.type, wi.parent_id, wi.path, wi.depth, wi.title,
  wi.notes, wi.is_completed, wi.completed_at, wi.completed_by, wi.bucket_id, wi.order_key,
  wi.assignee_id, wi.start_at, wi.due_at, wi.due_date_only, wi.due_time_zone,
  wi.cover_kind, wi.cover_color_token, wi.cover_media_id,
  -- The visible custom fields, exactly as FindWorkItem computes them and for the same reason.
  (SELECT coalesce(jsonb_object_agg(kv.key, kv.value), '{}'::jsonb)
     FROM jsonb_each(wi.custom_fields) AS kv
    WHERE EXISTS (
      SELECT 1 FROM custom_field_definition cfd
       WHERE cfd.deleted_at IS NULL
         AND cfd.id = (wi.custom_field_refs ->> kv.key)::uuid
         AND (cfd.collection_id = wi.collection_id OR cfd.collection_id IS NULL)
    ))::jsonb AS custom_fields,
  wi.content_language, wi.recurrence_rule_id,
  wi.archived_at, wi.deleted_at, wi.trash_batch_id, wi.created_by, wi.created_at, wi.updated_at,
  wi.version
FROM work_item wi
WHERE wi.path LIKE sqlc.arg('path_prefix')::text || '%'
  AND wi.id <> sqlc.arg('item_id')::uuid
  AND wi.deleted_at IS NULL
ORDER BY wi.depth, wi.order_key COLLATE "C", wi.id
LIMIT sqlc.arg('row_limit');

-- name: InsertWorkItemCopy :exec
-- A copy of an entry: a new row that carries the description of another one (C-11).
--
-- Its own statement rather than more columns on InsertWorkItem, because the two write different
-- things. A create writes what its use case owns and leaves every other column NULL, deliberately,
-- so that a field arrives through the use case that owns it; a copy writes the fields another row
-- already carries, all at once, because there is nothing to decide about them a second time and
-- writing them through five more statements would spend five versions on an entry that was born a
-- moment ago.
--
-- What is not here is what a copy does not carry: the completion, which names a person and a moment
-- and would be a false record on an entry that person never touched, the deletion stamps, and the
-- trash batch. The archive stamp is here, because an entry that was put away below the copied one
-- stays put away in the copy rather than being silently brought back.
--
-- The custom field document and its definition references arrive together and already resolved
-- against the destination: which definition a value belongs to is decided where the losses are
-- reported (I-W6), not here.
--
-- The columns no use case writes yet are absent: the recurrence rule and the jumble provenance.
-- They are NULL on every row this installation has, so carrying them would be copying a value
-- nothing can have set. Whichever milestone gives a column its first writer gives it a line here
-- in the same change - a copy that silently lost somebody's due date would be worse than the one
-- it lost. The schedule left that list with D-01, which is why its four columns are here.
INSERT INTO work_item (
  id, tenant_id, collection_id, type, parent_id, path, depth, title, notes,
  bucket_id, order_key, assignee_id,
  start_at, due_at, due_date_only, due_time_zone,
  cover_kind, cover_color_token, cover_media_id, custom_fields, custom_field_refs,
  content_language, archived_at, created_by, created_at, updated_at, version
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('collection_id'), sqlc.arg('type'),
  sqlc.narg('parent_id'), sqlc.arg('path'), sqlc.arg('depth'),
  normalize(sqlc.arg('title')::text, NFC),
  sqlc.narg('notes'), sqlc.narg('bucket_id'), sqlc.arg('order_key'),
  sqlc.narg('assignee_id'),
  sqlc.narg('start_at'), sqlc.narg('due_at'), coalesce(sqlc.narg('due_date_only'), false),
  sqlc.narg('due_time_zone'),
  sqlc.narg('cover_kind'), sqlc.narg('cover_color_token'), sqlc.narg('cover_media_id'),
  sqlc.arg('custom_fields')::jsonb, sqlc.arg('custom_field_refs')::jsonb,
  sqlc.narg('content_language'), sqlc.narg('archived_at'), sqlc.arg('created_by'),
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
  wi.id, wi.tenant_id, wi.collection_id, wi.type, wi.parent_id, wi.path, wi.depth, wi.title,
  wi.notes, wi.is_completed, wi.completed_at, wi.completed_by, wi.bucket_id, wi.order_key,
  wi.assignee_id, wi.start_at, wi.due_at, wi.due_date_only, wi.due_time_zone,
  wi.cover_kind, wi.cover_color_token, wi.cover_media_id,
  -- The visible custom fields, exactly as FindWorkItem computes them and for the same reason.
  (SELECT coalesce(jsonb_object_agg(kv.key, kv.value), '{}'::jsonb)
     FROM jsonb_each(wi.custom_fields) AS kv
    WHERE EXISTS (
      SELECT 1 FROM custom_field_definition cfd
       WHERE cfd.deleted_at IS NULL
         AND cfd.id = (wi.custom_field_refs ->> kv.key)::uuid
         AND (cfd.collection_id = wi.collection_id OR cfd.collection_id IS NULL)
    ))::jsonb AS custom_fields,
  wi.content_language, wi.recurrence_rule_id,
  wi.archived_at, wi.deleted_at, wi.trash_batch_id, wi.created_by, wi.created_at, wi.updated_at,
  wi.version
FROM work_item wi
WHERE wi.collection_id = sqlc.arg('collection_id')::uuid
  AND wi.parent_id IS NOT DISTINCT FROM sqlc.narg('parent_id')::uuid
  AND wi.deleted_at IS NULL
  AND (sqlc.arg('include_archived')::boolean OR wi.archived_at IS NULL)
  -- The entries the caller may see, when that is fewer than the level: null is no restriction at
  -- all, which is what every caller holding a role on the collection passes (C-04).
  AND (
    sqlc.narg('restrict_to')::uuid[] IS NULL
    OR wi.id = ANY(sqlc.narg('restrict_to')::uuid[])
  )
  AND (
    sqlc.narg('cursor_order_key')::text IS NULL
    OR (wi.order_key COLLATE "C", wi.id)
       > (sqlc.narg('cursor_order_key')::text COLLATE "C", sqlc.narg('cursor_id')::uuid)
  )
ORDER BY wi.order_key COLLATE "C", wi.id
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
-- Every column is written on every call, not only the ones that moved. The application has already
-- decided what the row should say - it read the item, applied the update and refused what the capability
-- profile does not allow - so this writes that decision whole. A statement that switched on which fields
-- were sent would be the second place deciding it, in the layer that is not allowed to decide anything
-- (ADR-0005), and `notes = COALESCE($1, notes)` would additionally make clearing the notes unexpressible.
--
-- Optimistic locking in the WHERE clause, as everywhere: the update matches nothing when somebody else has
-- moved the row on, and the caller learns that rather than overwriting them (api-guidelines.md §5).
--
-- `search_document` follows by itself, through the trigger migration 0019 puts on this table. The
-- index behind full text search therefore cannot fall behind a rename, and it cannot fall behind a
-- change of language either - which is what the trigger buys over the generated column it replaces
-- (ADR-0034): a generated column can only be a function of the row, and the configuration a
-- document is built under is not one PostgreSQL will let a generated column choose.
UPDATE work_item SET
  title            = sqlc.arg('title'),
  notes            = sqlc.narg('notes'),
  bucket_id        = sqlc.narg('bucket_id'),
  start_at         = sqlc.narg('start_at'),
  content_language = sqlc.narg('content_language'),
  updated_at       = sqlc.arg('updated_at'),
  version          = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: SetWorkItemDueDate :execrows
-- The due trio, set or cleared whole, under the same optimistic lock every write to this row
-- takes (api-guidelines.md §5).
--
-- Its own statement rather than columns added to SetWorkItemAttributes, for the reason the
-- assignee has one: a due date is one decision about one date, and a statement that wrote the
-- title alongside it would make moving a deadline spend the version of a rename nobody asked
-- for. The three columns travel together because none of them means anything alone (D-01,
-- i18n-l10n.md §4) - which fields *moved* is the application's answer, recorded in the change
-- log per field; the row simply says what is now true.
--
-- The two announcement stamps are cleared with it (D-03): a date that moves is a new deadline,
-- which may be approached and missed again, and a stamp left standing would silence the
-- announcement for it. Cleared here rather than by the caller, because they are bookkeeping about
-- this column and nothing outside this statement writes it.
UPDATE work_item SET
  due_at                = sqlc.narg('due_at'),
  due_date_only         = coalesce(sqlc.narg('due_date_only'), false),
  due_time_zone         = sqlc.narg('due_time_zone'),
  due_soon_announced_at = NULL,
  overdue_announced_at  = NULL,
  updated_at            = sqlc.arg('updated_at'),
  version               = version + 1
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

-- name: CountOpenItemsByAssignee :many
-- LEAST_LOADED's material (C-02): how many open entries each candidate carries, tenant-wide,
-- because a person's load does not stop at a collection's edge. Open means not completed, not in
-- the trash, and not archived by its own stamp - the inherited archive of an ancestor is not
-- consulted, which overcounts a dormant subtree's entries rather than paying a recursive walk on
-- every create. Candidates with no open entry are simply absent from the answer; the caller reads
-- an absent key as zero.
SELECT assignee_id, COUNT(*) AS open_items
FROM work_item
WHERE assignee_id = ANY(sqlc.arg('account_ids')::uuid[])
  AND is_completed = false
  AND deleted_at IS NULL
  AND archived_at IS NULL
GROUP BY assignee_id;

-- name: SetWorkItemCover :execrows
-- The cover, set or cleared, under the same optimistic lock every write to this row takes. Its
-- own statement for the reason the assignee has one: a cover is one decision about one field,
-- and spending a rename's version on it would be a version nobody asked for. The consistency of
-- the three columns is the table's CHECK (migration 0013), so a statement that half-set a cover
-- would be refused by the database.
UPDATE work_item SET
  cover_kind        = sqlc.narg('cover_kind'),
  cover_color_token = sqlc.narg('cover_color_token'),
  cover_media_id    = sqlc.narg('cover_media_id'),
  updated_at        = sqlc.arg('updated_at'),
  version           = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: SetWorkItemCustomField :execrows
-- One key of the entry's custom field document, under the same optimistic lock every write to this
-- row takes. One key rather than the whole document, and that is a data-safety decision as much as
-- a merge one: the row may hold values whose definitions were deleted - visible to no read, but
-- kept - and a write that replaced the document with what a read answered would erase them. The
-- ref travels in the same statement, because a value and the identity of the definition it was
-- written under are one fact: written apart, a crash between the two would leave a value no read
-- can ever judge. A NULL value removes the key and its ref - "cleared" has one spelling, since the
-- reads cannot tell a stored null from an absent key. The version predicate is what makes two
-- devices writing two different keys resolve rather than overwrite.
UPDATE work_item SET
  custom_fields = CASE
    WHEN sqlc.narg('value')::jsonb IS NULL THEN custom_fields - sqlc.arg('key')::text
    ELSE jsonb_set(custom_fields, ARRAY[sqlc.arg('key')::text], sqlc.narg('value')::jsonb, true)
  END,
  custom_field_refs = CASE
    WHEN sqlc.narg('value')::jsonb IS NULL THEN custom_field_refs - sqlc.arg('key')::text
    ELSE jsonb_set(custom_field_refs, ARRAY[sqlc.arg('key')::text],
                   to_jsonb(sqlc.arg('definition_id')::uuid::text), true)
  END,
  updated_at = sqlc.arg('updated_at'),
  version    = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: ClaimDueSoonItems :many
-- The entries whose deadline has come within the lead and have not been announced yet, claimed and
-- stamped in one statement (D-03).
--
-- One statement rather than a select and an update, which is what makes the announcement
-- exactly-once: the stamp is written by the same UPDATE that returns the row, so two passes over
-- the same entry - another leader, a retried job - cannot both see it as unannounced. The same
-- reasoning as the reminder's guarded transition, in the shape a scan needs.
--
-- Only open entries: something completed, trashed or archived is not approaching anything, which
-- is also what the index this runs on is partial over.
UPDATE work_item SET due_soon_announced_at = sqlc.arg('now')
WHERE id IN (
  SELECT due.id FROM work_item AS due
  WHERE due.due_at IS NOT NULL
    AND due.due_at <= sqlc.arg('threshold')
    AND due.due_soon_announced_at IS NULL
    AND due.deleted_at IS NULL AND due.archived_at IS NULL AND due.is_completed = false
  ORDER BY due.due_at
  FOR UPDATE SKIP LOCKED
  LIMIT sqlc.arg('batch_size')
)
RETURNING id, tenant_id, collection_id, due_at, due_date_only, due_time_zone;

-- name: ClaimOverdueItems :many
-- The same scan for the deadline itself: entries whose date has passed with the work not done.
UPDATE work_item SET overdue_announced_at = sqlc.arg('now')
WHERE id IN (
  SELECT due.id FROM work_item AS due
  WHERE due.due_at IS NOT NULL
    AND due.due_at <= sqlc.arg('now')
    AND due.overdue_announced_at IS NULL
    AND due.deleted_at IS NULL AND due.archived_at IS NULL AND due.is_completed = false
  ORDER BY due.due_at
  FOR UPDATE SKIP LOCKED
  LIMIT sqlc.arg('batch_size')
)
RETURNING id, tenant_id, collection_id, due_at, due_date_only, due_time_zone;

-- name: NextDueAnnouncement :one
-- When this tenant next owes an announcement: the lead before a deadline that has not been
-- announced as approaching, or the deadline itself where only the overdue announcement is left.
-- NULL when it owes none, which is half of what lets the firing job finish (D-03).
SELECT min(
  CASE WHEN due_soon_announced_at IS NULL
       THEN due_at - make_interval(secs => sqlc.arg('lead_seconds')::double precision)
       ELSE due_at
  END
)::timestamptz AS next_at
FROM work_item
WHERE due_at IS NOT NULL
  AND deleted_at IS NULL AND archived_at IS NULL AND is_completed = false
  AND (due_soon_announced_at IS NULL OR overdue_announced_at IS NULL);
