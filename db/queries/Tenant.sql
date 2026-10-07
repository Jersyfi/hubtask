
-- name: InsertTenantHost :exec
-- A host a workspace answers at. `current_tenant_id()` rather than an argument, as every
-- tenant-scoped insert here: the workspace is the transaction's, not the caller's to name.
INSERT INTO tenant_host
  (tenant_id, host, state, verification, verified_at, is_canonical, created_at)
VALUES (
  current_tenant_id(), sqlc.arg('host'), sqlc.arg('state'), sqlc.arg('verification'),
  sqlc.narg('verified_at'), sqlc.arg('is_canonical'), sqlc.arg('created_at')
);

-- name: ListTenantHosts :many
-- The hosts of this workspace, canonical first - what a reader needs to know which one a mail will
-- name. Nothing resolves a request through this yet (migration 0104).
SELECT host, state, verification, verified_at, is_canonical, created_at
FROM tenant_host
ORDER BY is_canonical DESC, host;
