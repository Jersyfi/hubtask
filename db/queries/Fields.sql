-- Custom field definitions (C-07).
--
-- The values are not here: they live in `work_item.custom_fields`, a jsonb document on the entry,
-- and are written by the statement beside the entry's own attributes (Work.sql,
-- SetWorkItemCustomFields). What these statements maintain is the vocabulary - which keys exist in
-- which scope, what they may hold, and which item types carry them.

-- name: InsertCustomField :exec
-- The unique index cfd_key_uq decides whether the key is free, rather than a check followed by an
-- insert: two requests arriving in the gap between the two would both pass the check.
INSERT INTO custom_field_definition (
  id, tenant_id, collection_id, key, kind, options, is_required, applies_to,
  created_at, updated_at, version
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.narg('collection_id'), sqlc.arg('key'),
  sqlc.arg('kind'), sqlc.arg('options'), sqlc.arg('is_required'),
  sqlc.arg('applies_to')::item_type[], sqlc.arg('created_at'), sqlc.arg('created_at'), 1
);

-- name: FindCustomField :one
SELECT id, tenant_id, collection_id, key, kind, options, is_required, applies_to,
       created_at, updated_at, deleted_at, version
FROM custom_field_definition
WHERE id = $1;

-- name: FindCustomFieldInScope :one
-- The definition one entry's collection sees under a key: its collection's own, or the
-- workspace-wide one. The collection's own wins, which is what makes a workspace-wide default
-- overridable by a team that needs something narrower - `ORDER BY` puts the specific one first and
-- the row limit takes it.
SELECT id, tenant_id, collection_id, key, kind, options, is_required, applies_to,
       created_at, updated_at, deleted_at, version
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
SELECT id, tenant_id, collection_id, key, kind, options, is_required, applies_to,
       created_at, updated_at, deleted_at, version
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
