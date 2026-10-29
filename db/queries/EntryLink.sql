-- Links and references (migration 0213, F-3, UC-PLN-01). A workspace's own link types only: the
-- six shipped ones are the domain's and have no row.

-- name: ListLinkTypes :many
SELECT id, tenant_id, key, name, inverse_name, direction, cardinality, cycles, waits, position,
  archived_at, created_at, updated_at, version
FROM link_type
WHERE tenant_id = current_tenant_id()
ORDER BY position, key;

-- name: FindLinkType :one
SELECT id, tenant_id, key, name, inverse_name, direction, cardinality, cycles, waits, position,
  archived_at, created_at, updated_at, version
FROM link_type
WHERE tenant_id = current_tenant_id() AND key = sqlc.arg('key')::text;

-- name: InsertLinkType :exec
INSERT INTO link_type (
  id, tenant_id, key, name, inverse_name, direction, cardinality, cycles, waits, position,
  archived_at, created_at, updated_at, version
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('key'), normalize(sqlc.arg('name')::text, NFC),
  normalize(sqlc.arg('inverse_name')::text, NFC), sqlc.arg('direction'), sqlc.arg('cardinality'),
  sqlc.arg('cycles'), sqlc.arg('waits'), sqlc.arg('position'), sqlc.narg('archived_at'),
  sqlc.arg('created_at'), sqlc.arg('created_at'), 1
);

-- name: UpdateLinkType :execrows
-- What a type's meaning is (direction, cardinality, cycles, waits) never changes once links stand.
UPDATE link_type SET
  name         = normalize(sqlc.arg('name')::text, NFC),
  inverse_name = normalize(sqlc.arg('inverse_name')::text, NFC),
  position     = sqlc.arg('position'),
  archived_at  = sqlc.narg('archived_at'),
  updated_at   = sqlc.arg('updated_at'),
  version      = version + 1
WHERE tenant_id = current_tenant_id() AND key = sqlc.arg('key')::text
  AND version = sqlc.arg('expected_version')::int;

-- name: DeleteLinkType :execrows
-- A type a link still names is held by entry_link_type_fkey.
DELETE FROM link_type WHERE tenant_id = current_tenant_id() AND key = sqlc.arg('key')::text;

-- name: LinkTypeInUse :one
SELECT EXISTS (
  SELECT 1 FROM entry_link WHERE tenant_id = current_tenant_id() AND own_type = sqlc.arg('key')::text
) AS in_use;

-- name: LockEntryLinks :exec
-- One lock per workspace for the length of a link's write: the cardinality and the cycle rule are
-- decided over links other transactions may be drawing at the same moment.
SELECT pg_advisory_xact_lock(hashtextextended('entry_link/' || current_tenant_id()::text, 0));

-- name: InsertEntryLink :exec
INSERT INTO entry_link (id, tenant_id, link_type, source_id, target_id, created_by, created_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('link_type'), sqlc.arg('source_id'),
  sqlc.arg('target_id'), sqlc.narg('created_by'), sqlc.arg('created_at'));

-- name: FindEntryLink :one
SELECT id, tenant_id, link_type, source_id, target_id, created_by, created_at
FROM entry_link
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: DeleteEntryLink :execrows
DELETE FROM entry_link WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: ListEntryLinksOf :many
-- An entry's links from both ends, oldest first, bounded.
SELECT id, tenant_id, link_type, source_id, target_id, created_by, created_at
FROM entry_link
WHERE tenant_id = current_tenant_id()
  AND (source_id = sqlc.arg('entry_id')::uuid OR target_id = sqlc.arg('entry_id')::uuid)
ORDER BY created_at, id
LIMIT sqlc.arg('max')::int;

-- name: ListEntryLinksTouching :many
-- The links with at least one end among the entries, for a timeline or a rule's `item.links`.
SELECT id, tenant_id, link_type, source_id, target_id, created_by, created_at
FROM entry_link
WHERE tenant_id = current_tenant_id()
  AND (source_id = ANY(sqlc.arg('entry_ids')::uuid[]) OR target_id = ANY(sqlc.arg('entry_ids')::uuid[]))
ORDER BY created_at, id
LIMIT sqlc.arg('max')::int;

-- name: CountEntryLinksFrom :one
SELECT count(*)::int AS links
FROM entry_link
WHERE tenant_id = current_tenant_id() AND source_id = sqlc.arg('source_id')::uuid
  AND link_type = sqlc.arg('link_type')::text;

-- name: EntryLinkPathExists :one
-- Whether `goal` is reachable from `start` over the links of the `forward` types read source to
-- target and of the `backward` types read target to source: a new link closes a loop when it is.
-- UNION, not UNION ALL, so a graph that already loops (a type that allows it) still ends.
WITH RECURSIVE edges AS (
  SELECT l.source_id AS from_id, l.target_id AS to_id
    FROM entry_link l
   WHERE l.tenant_id = current_tenant_id() AND l.link_type = ANY(sqlc.arg('forward')::text[])
  UNION ALL
  SELECT l.target_id, l.source_id
    FROM entry_link l
   WHERE l.tenant_id = current_tenant_id() AND l.link_type = ANY(sqlc.arg('backward')::text[])
), reach (id) AS (
  SELECT sqlc.arg('start')::uuid
  UNION
  SELECT e.to_id FROM edges e JOIN reach r ON e.from_id = r.id
)
SELECT EXISTS (SELECT 1 FROM reach WHERE id = sqlc.arg('goal')::uuid) AS reaches;

-- name: ListEntrySummaries :many
-- What a link's or a reference's end shows - its title, number and state - with the collection and
-- the hub its reader's permission is judged on. Trashed entries are left out: they are not there.
SELECT wi.id, wi.collection_id, c.parent_id AS hub_id, c.handle, wi.type::text AS level,
  wi.entry_type, wi.title, wi.number, wi.is_completed, wi.start_at, wi.due_at, wi.archived_at
FROM work_item wi
JOIN container c ON c.tenant_id = wi.tenant_id AND c.id = wi.collection_id
WHERE wi.tenant_id = current_tenant_id() AND wi.id = ANY(sqlc.arg('ids')::uuid[])
  AND wi.deleted_at IS NULL AND c.deleted_at IS NULL;

-- name: ListContainerSummaries :many
SELECT c.id, c.type::text AS kind, c.parent_id, c.name, c.handle, c.archived_at
FROM container c
WHERE c.tenant_id = current_tenant_id() AND c.id = ANY(sqlc.arg('ids')::uuid[])
  AND c.deleted_at IS NULL;

-- name: ListBacklinkReferences :many
-- The texts that point at an object, with the entry each belongs to, bounded.
SELECT source_kind, source_id, entry_id, anchor, strength
FROM entry_reference
WHERE tenant_id = current_tenant_id() AND target_kind = sqlc.arg('target_kind')::text
  AND target_id = sqlc.arg('target_id')::uuid
ORDER BY entry_id, source_kind, source_id
LIMIT sqlc.arg('max')::int;

-- name: ListEntryReferencesFrom :many
-- What one text points at: an entry's notes or a comment.
SELECT target_kind, target_id, anchor, strength
FROM entry_reference
WHERE tenant_id = current_tenant_id() AND source_kind = sqlc.arg('source_kind')::text
  AND source_id = sqlc.arg('source_id')::uuid
ORDER BY target_kind, target_id, anchor;
