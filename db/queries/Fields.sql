-- Custom field definitions.
--
-- The values are not here: they live in `work_item.custom_fields`, a jsonb document on the entry,
-- and are written by the statement beside the entry's own attributes (Work.sql,
-- SetWorkItemCustomFields). What these statements maintain is the vocabulary - which keys exist in
-- which scope, what they may hold, and which item types carry them.

-- name: InsertCustomField :exec
-- The unique index cfd_key_uq decides whether the key is free, rather than a check followed by an
-- insert: two requests arriving in the gap between the two would both pass the check.
INSERT INTO custom_field_definition (
  id, tenant_id, collection_id, key, kind, options, currency, formula, is_required, restricted, read_restricted, applies_to,
  created_at, updated_at, version
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.narg('collection_id'), sqlc.arg('key'),
  sqlc.arg('kind'), sqlc.arg('options'), sqlc.narg('currency'), sqlc.narg('formula'), sqlc.arg('is_required'),
  sqlc.arg('restricted'), sqlc.arg('read_restricted'), sqlc.arg('applies_to')::item_type[], sqlc.arg('created_at'), sqlc.arg('created_at'), 1
);

-- name: FindCustomField :one
SELECT id, tenant_id, collection_id, key, kind, options, currency, formula, is_required, applies_to,
       created_at, updated_at, deleted_at, version, restricted, read_restricted
FROM custom_field_definition
WHERE id = $1;

-- name: FindCustomFieldInScope :one
-- The definition one entry's collection sees under a key: its collection's own, or the
-- workspace-wide one. The collection's own wins, which is what makes a workspace-wide default
-- overridable by a team that needs something narrower - `ORDER BY` puts the specific one first and
-- the row limit takes it.
SELECT id, tenant_id, collection_id, key, kind, options, currency, formula, is_required, applies_to,
       created_at, updated_at, deleted_at, version, restricted, read_restricted
FROM custom_field_definition
WHERE deleted_at IS NULL
  AND key = sqlc.arg('key')
  AND (collection_id = sqlc.narg('collection_id')::uuid OR collection_id IS NULL)
ORDER BY collection_id NULLS LAST
LIMIT 1;

-- name: ListCustomFieldsInScope :many
-- Every definition in force for one collection: its own and the workspace-wide ones above it. A
-- NULL collection answers the workspace-wide ones alone, which is what a client configuring the
-- workspace asks for. Ordered by scope and then by key, so the answer is stable and the specific
-- definitions sit beside the general ones they narrow.
SELECT id, tenant_id, collection_id, key, kind, options, currency, formula, is_required, applies_to,
       created_at, updated_at, deleted_at, version, restricted, read_restricted
FROM custom_field_definition
WHERE deleted_at IS NULL
  AND (collection_id = sqlc.narg('collection_id')::uuid OR collection_id IS NULL)
ORDER BY collection_id NULLS FIRST, key;

-- name: UpdateCustomField :execrows
-- What an edit may change, under the optimistic lock every editable row takes. The key and the
-- kind are not in the statement at all: a key that moved would orphan every value stored under it,
-- and a kind that changed would reinterpret them.
UPDATE custom_field_definition SET
  options     = sqlc.arg('options'),
  is_required = sqlc.arg('is_required'),
  restricted  = sqlc.arg('restricted'),
  read_restricted = sqlc.arg('read_restricted'),
  applies_to  = sqlc.arg('applies_to')::item_type[],
  updated_at  = sqlc.arg('updated_at'),
  version     = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version') AND deleted_at IS NULL;

-- name: SoftDeleteCustomField :execrows
-- A soft delete. The values stay in the entries and stop being visible: rewriting `custom_fields`
-- across every entry in a collection would be an unbounded write from one request, and a
-- definition recreated under the same key must not resurrect what the old one held - which the
-- partial unique index makes possible by ignoring the deleted row.
UPDATE custom_field_definition SET
  deleted_at = sqlc.arg('deleted_at'),
  updated_at = sqlc.arg('deleted_at'),
  version    = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version') AND deleted_at IS NULL;

-- name: ListCustomFieldKindsByKey :many
-- The kinds the live definitions under these keys have, in force anywhere a query's scope reaches:
-- the workspace-wide ones and those of the scope's collections - the collection itself, or every
-- collection of a hub. A NULL scope is the whole workspace, which is what a search spans. What the
-- query language needs to know which typed column a range, a sort or a grouping reads (F-2); a key
-- with two kinds in one scope answers both, and the use case refuses to guess.
SELECT DISTINCT d.key, d.kind
FROM custom_field_definition d
WHERE d.deleted_at IS NULL
  AND d.key = ANY(sqlc.arg('keys')::text[])
  AND (sqlc.narg('scope')::uuid IS NULL
       OR d.collection_id IS NULL
       OR d.collection_id = sqlc.narg('scope')::uuid
       OR d.collection_id IN (SELECT c.id FROM container c WHERE c.parent_id = sqlc.narg('scope')::uuid))
ORDER BY d.key, d.kind;

-- name: ListReadRestrictedFields :many
-- The definitions hidden from readers, across the workspace: what every read asks before it
-- narrows an answer (UC-ID-27 check 3). Small - a handful per workspace - and empty for most.
SELECT key, collection_id
FROM custom_field_definition
WHERE deleted_at IS NULL AND read_restricted
ORDER BY key, collection_id NULLS FIRST;

-- name: ListFormulaFields :many
-- The CALCULATED definitions and those a formula may read, across the workspace: asked only where
-- a field is hidden from readers, to hide with it a calculated field that reads it.
SELECT key, collection_id, kind, formula
FROM custom_field_definition
WHERE deleted_at IS NULL AND kind IN ('CALCULATED', 'NUMBER', 'MONEY', 'DURATION', 'BOOL')
ORDER BY key, collection_id NULLS FIRST;

-- name: CountEntriesHoldingCustomField :one
-- The live entries that hold a value under a definition's key where the definition is in force:
-- its collection, or for a workspace-wide one every collection that does not define the key
-- itself. What removing the definition or hiding it from readers would stop showing - a count,
-- never a value (rule 10).
SELECT count(*)::bigint
FROM work_item wi
WHERE wi.deleted_at IS NULL
  AND wi.custom_fields ? sqlc.arg('key')::text
  AND (CASE WHEN sqlc.narg('collection_id')::uuid IS NULL
         THEN NOT EXISTS (SELECT 1 FROM custom_field_definition own
                          WHERE own.deleted_at IS NULL AND own.key = sqlc.arg('key')::text
                            AND own.collection_id = wi.collection_id)
         ELSE wi.collection_id = sqlc.narg('collection_id')::uuid
       END);
