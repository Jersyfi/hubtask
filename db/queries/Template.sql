-- The templates (domain-model.md §3.5).
--
-- The tenant is never a parameter here: it comes from the transaction's own context through
-- current_tenant_id(), which is the same value row level security compares against (ADR-0010).
--
-- The tree is stored as the document it is. Nothing here interprets it: which type may sit under
-- which is the hierarchy's answer and is decided before a row is written, and what a relative date
-- means is decided when the template is stamped out.

-- name: InsertTemplate :exec
INSERT INTO template (
  id, tenant_id, scope_type, scope_id, name, description, root_type, nodes, created_at, version
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('scope_type'), sqlc.narg('scope_id'),
  normalize(sqlc.arg('name')::text, NFC), sqlc.narg('description'),
  sqlc.arg('root_type')::item_type, sqlc.arg('nodes')::jsonb, sqlc.arg('created_at'), 1
);

-- name: FindTemplate :one
-- A deleted template is returned rather than filtered out: whether it may still be changed is the
-- domain's question, and a query that hid one would turn "it was deleted" into "it never existed" -
-- which is not what a client holding its identifier is asking.
SELECT id, tenant_id, scope_type, scope_id, name, description, root_type, nodes,
       created_at, updated_at, deleted_at, version
FROM template
WHERE id = $1;

-- name: ListTemplatesInScopes :many
-- The templates a person picking one in a container can choose from: the ones defined on that
-- container's path, plus the workspace-wide ones. The caller passes the identifiers the path names
-- (the collection, its hub) and TENANT-scoped rows match by scope type alone, having no identifier
-- to match - the shape the saved views' reachable list already has.
--
-- Newest first: a template somebody has just written is the one they are looking for. The keyset
-- is (created_at, id) for the reason every list in this schema takes the pair.
SELECT id, tenant_id, scope_type, scope_id, name, description, root_type, nodes,
       created_at, updated_at, deleted_at, version
FROM template
WHERE deleted_at IS NULL
  AND (
    scope_type = 'TENANT'
    OR scope_id = ANY (sqlc.arg('scope_ids')::uuid[])
  )
  AND (
    sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('page_size');

-- name: UpdateTemplate :execrows
-- The whole document, under the same optimistic lock every other row takes (api-guidelines.md §5).
-- The scope and the root type are not here on purpose: a template that changed scope would move
-- out from under the people who could use it, and one whose root type changed would produce a
-- different kind of thing under the same name.
UPDATE template SET
  name        = normalize(sqlc.arg('name')::text, NFC),
  description = sqlc.narg('description'),
  nodes       = sqlc.arg('nodes')::jsonb,
  updated_at  = sqlc.arg('updated_at'),
  version     = version + 1
WHERE id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')
  AND deleted_at IS NULL;

-- name: SetTemplateDeleted :execrows
-- The soft delete. The trees the template has stamped out are ordinary entries and are not
-- touched; what goes is the ability to stamp out more - and the name, which the partial unique
-- index frees for a template defined afterwards.
UPDATE template SET
  deleted_at = sqlc.arg('deleted_at'),
  version    = version + 1
WHERE id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')
  AND deleted_at IS NULL;

-- name: RemoveTemplate :execrows
-- The deletion where no legal hold covers the template: the row goes at once (data-retention.md
-- §4). Nothing shows a deleted template's name and nothing references one, so there is no
-- tombstone row to keep; the journal and the tombstone the caller writes are the record. Under the
-- same optimistic lock as the edit, and never on a row already deleted - that one is the hold's.
DELETE FROM template
WHERE id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')
  AND deleted_at IS NULL;

-- name: DeletedTemplates :many
-- The templates deleted under a legal hold, with what a hold is judged against: the scope and, for
-- a collection, the hub above it. One page in identifier order; the retention pass walks every
-- page, because a template still held stays in the set.
SELECT t.id, t.scope_type, t.scope_id, c.parent_id AS hub_id
FROM template t
LEFT JOIN container c ON c.tenant_id = t.tenant_id AND c.id = t.scope_id
WHERE t.tenant_id = current_tenant_id() AND t.deleted_at IS NOT NULL
  AND t.id > sqlc.arg('after')::uuid
ORDER BY t.id
LIMIT sqlc.arg('batch');

-- name: RemoveDeletedTemplates :many
-- The held templates no hold covers any more, removed. The identifiers come back so that the
-- journal names exactly the rows that went. A living template is never matched.
DELETE FROM template
WHERE tenant_id = current_tenant_id() AND deleted_at IS NOT NULL
  AND id = ANY(sqlc.arg('ids')::uuid[])
RETURNING id;
