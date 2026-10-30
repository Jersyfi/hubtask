-- Releases and their entries (migration 0272, M2, PLN-08).

-- name: InsertRelease :exec
INSERT INTO release (id, tenant_id, hub_id, name, description, release_on, state, created_at,
  created_by, updated_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('hub_id'),
  normalize(sqlc.arg('name')::text, NFC), normalize(sqlc.arg('description')::text, NFC),
  sqlc.arg('release_on'), 'PLANNED', sqlc.arg('created_at'), sqlc.narg('created_by'),
  sqlc.arg('created_at'), 1);

-- name: UpdateRelease :execrows
-- Every field a person or a release changes, held to the version the caller read.
UPDATE release SET
  name = normalize(sqlc.arg('name')::text, NFC),
  description = normalize(sqlc.arg('description')::text, NFC),
  release_on = sqlc.arg('release_on'), state = sqlc.arg('state'),
  released_at = sqlc.narg('released_at'), notes_page_id = sqlc.narg('notes_page_id'),
  updated_at = sqlc.arg('updated_at'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')::int;

-- name: FindRelease :one
SELECT id, hub_id, name, description, release_on, state, released_at, notes_page_id, created_at,
  created_by, updated_at, version
FROM release
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: ListReleases :many
-- A hub's releases, the latest day first, of one state or all, bounded.
SELECT id, hub_id, name, description, release_on, state, released_at, notes_page_id, created_at,
  created_by, updated_at, version
FROM release
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id')::uuid
  AND (sqlc.arg('state')::text = '' OR state = sqlc.arg('state')::text)
ORDER BY release_on DESC, id
LIMIT sqlc.arg('max')::int;

-- name: DeleteRelease :execrows
DELETE FROM release WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: ListReleaseEntries :many
-- A release's entries with what its notes and a reader's view need: the title, the type's name,
-- the completion and where the entry lives; a trashed entry is left out.
SELECT re.entry_id, re.added_at, wi.title, wi.is_completed, wi.collection_id, c.parent_id AS hub_id,
  coalesce(et.name, '')::text AS type_name
FROM release_entry re
JOIN work_item wi ON wi.tenant_id = re.tenant_id AND wi.id = re.entry_id
JOIN container c ON c.tenant_id = wi.tenant_id AND c.id = wi.collection_id
LEFT JOIN entry_type et ON et.tenant_id = wi.tenant_id AND et.key = wi.entry_type
WHERE re.tenant_id = current_tenant_id() AND re.release_id = sqlc.arg('release_id')::uuid
  AND wi.deleted_at IS NULL
ORDER BY re.added_at, re.entry_id
LIMIT sqlc.arg('max')::int;

-- name: CountReleaseEntries :one
SELECT count(*)::int FROM release_entry
WHERE tenant_id = current_tenant_id() AND release_id = sqlc.arg('release_id')::uuid;

-- name: ListEntryReleases :many
-- Which release each of the entries is in.
SELECT entry_id, release_id
FROM release_entry
WHERE tenant_id = current_tenant_id() AND entry_id = ANY(sqlc.arg('entry_ids')::uuid[]);

-- name: AddReleaseEntry :exec
INSERT INTO release_entry (tenant_id, release_id, entry_id, added_at)
VALUES (current_tenant_id(), sqlc.arg('release_id'), sqlc.arg('entry_id'), sqlc.arg('added_at'))
ON CONFLICT (tenant_id, release_id, entry_id) DO NOTHING;

-- name: RemoveReleaseEntry :execrows
DELETE FROM release_entry
WHERE tenant_id = current_tenant_id() AND release_id = sqlc.arg('release_id')::uuid
  AND entry_id = sqlc.arg('entry_id')::uuid;
