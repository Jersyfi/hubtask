-- Saved views (D-07).
--
-- The query column is stored exactly as the client sent it, validated by the application against
-- the same grammar the query endpoint applies. Nothing here interprets it - or the layout, or the
-- visible fields: this table is a bookmark shelf, and what a bookmark opens into is decided by
-- whoever opens it, under their own authorisation.

-- name: InsertSavedView :exec
INSERT INTO saved_view (
  id, tenant_id, scope_type, scope_id, owner_id, name, layout, query, grouping,
  visible_fields, sharing, created_at, version
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('scope_type'), sqlc.narg('scope_id'),
  sqlc.arg('owner_id'),
  normalize(sqlc.arg('name')::text, NFC),
  sqlc.arg('layout'), sqlc.arg('query')::jsonb, sqlc.arg('grouping')::jsonb,
  sqlc.arg('visible_fields')::text[], sqlc.arg('sharing'), sqlc.arg('created_at'), 1
);

-- name: FindSavedView :one
SELECT id, tenant_id, scope_type, scope_id, owner_id, name, layout, query, grouping,
       visible_fields, sharing, created_at, version
FROM saved_view
WHERE id = $1;

-- name: ListSavedViewsOwned :many
-- The caller's own views, whatever their scope and sharing. Ordered by creation and then by
-- identifier, so the answer is stable without an updated_at this table deliberately does not
-- carry.
SELECT id, tenant_id, scope_type, scope_id, owner_id, name, layout, query, grouping,
       visible_fields, sharing, created_at, version
FROM saved_view
WHERE owner_id = sqlc.arg('owner_id')::uuid
ORDER BY created_at, id;

-- name: ListSavedViewsReachable :many
-- The caller's own views plus what is shared along one container's path: the caller passes the
-- scope identifiers that path names - the collection, its hub - and TENANT-scoped shares match by
-- scope type alone, having no identifier to match. The authorisation happened before this runs
-- (one question about the container, ADR-0005); the array is its answer bound into the statement,
-- never a filter after the page.
SELECT id, tenant_id, scope_type, scope_id, owner_id, name, layout, query, grouping,
       visible_fields, sharing, created_at, version
FROM saved_view
WHERE owner_id = sqlc.arg('owner_id')::uuid
   OR (sharing = 'SCOPE' AND (
        scope_type = 'TENANT'
        OR scope_id = ANY (sqlc.arg('scope_ids')::uuid[])
      ))
ORDER BY created_at, id;

-- name: SetSavedViewAttributes :execrows
-- A view's own fields, written whole under the optimistic lock every editable row takes. The
-- scope and the sharing are not in the statement: where a view lives is decided at creation, and
-- sharing has a statement of its own - one decision about one field, never spending a rename's
-- version.
UPDATE saved_view SET
  name           = normalize(sqlc.arg('name')::text, NFC),
  layout         = sqlc.arg('layout'),
  query          = sqlc.arg('query')::jsonb,
  grouping       = sqlc.arg('grouping')::jsonb,
  visible_fields = sqlc.arg('visible_fields')::text[],
  version        = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: SetSavedViewSharing :execrows
UPDATE saved_view SET
  sharing = sqlc.arg('sharing'),
  version = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: DeleteSavedView :execrows
-- A hard delete: the table carries no deleted_at, and a view is a bookmark rather than content -
-- nothing below it to keep, nothing to restore. A calendar feed that served it keeps its row and
-- loses the reference, which is the composite foreign key's ON DELETE SET NULL (migration 0005)
-- and D-08's question answered here.
DELETE FROM saved_view
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');
