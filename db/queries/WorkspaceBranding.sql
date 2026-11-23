-- A workspace's own name and logo (UC-INS-22, migration 0449): its pictures, one row each, and what
-- its header shows, the settings' `header_shows`. Nothing here names a tenant: row level security
-- binds every statement to the transaction's workspace.

-- name: ListWorkspacePictures :many
SELECT picture, media_id FROM workspace_picture ORDER BY picture;

-- name: LockWorkspaceHeader :one
-- What the header shows, read for a change to the branding and holding the workspace's row until
-- the transaction ends: two administrators setting two pictures at once must not each decide the
-- choice from what the other is about to change. The product's name where the workspace never chose.
SELECT COALESCE(settings->>'header_shows', 'PRODUCT')::text AS header_shows
FROM tenant
WHERE id = current_tenant_id() AND deleted_at IS NULL
FOR UPDATE;

-- name: SetWorkspacePicture :exec
INSERT INTO workspace_picture (tenant_id, picture, media_id)
VALUES (current_tenant_id(), sqlc.arg('picture'), sqlc.arg('media_id'))
ON CONFLICT (tenant_id, picture) DO UPDATE SET media_id = EXCLUDED.media_id;

-- name: RemoveWorkspacePicture :exec
DELETE FROM workspace_picture WHERE picture = sqlc.arg('picture');

-- name: SetWorkspaceHeader :execrows
-- Merged into the settings document, so every other key stays as it is; not the row version, which
-- guards the fields of the workspace's PATCH. `updated_at` moves, so that an incremental archive
-- carries the change.
UPDATE tenant
SET settings = settings || jsonb_build_object('header_shows', sqlc.arg('header_shows')::text),
    updated_at = sqlc.arg('now')
WHERE id = current_tenant_id() AND deleted_at IS NULL;
