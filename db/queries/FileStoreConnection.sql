-- A person's own sign-in to a file store (migration 0279, UC-INT-17).

-- name: ListFileStoreConnections :many
SELECT id, account_id, kind, server, login, created_at, used_at
FROM file_store_connection
WHERE tenant_id = current_tenant_id() AND account_id = sqlc.arg('account_id')
ORDER BY created_at, id;

-- name: CountFileStoreConnections :one
SELECT count(*)::int AS connections FROM file_store_connection
WHERE tenant_id = current_tenant_id() AND account_id = sqlc.arg('account_id');

-- name: FindFileStoreConnection :one
SELECT id, account_id, kind, server, login, created_at, used_at, secret_enc, secret_key_id
FROM file_store_connection
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: UpsertFileStoreConnection :one
INSERT INTO file_store_connection (id, tenant_id, account_id, kind, server, login, secret_enc, secret_key_id, created_at, used_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('kind'), sqlc.arg('server'), sqlc.arg('login'),
        sqlc.arg('secret_enc'), sqlc.arg('secret_key_id'), sqlc.arg('created_at'), sqlc.arg('created_at'))
ON CONFLICT (tenant_id, account_id, kind, server, login) DO UPDATE
SET secret_enc = EXCLUDED.secret_enc, secret_key_id = EXCLUDED.secret_key_id, used_at = EXCLUDED.used_at
RETURNING id;

-- name: UpdateFileStoreConnectionSecret :exec
UPDATE file_store_connection SET secret_enc = sqlc.arg('secret_enc'), secret_key_id = sqlc.arg('secret_key_id')
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: TouchFileStoreConnection :exec
UPDATE file_store_connection SET used_at = sqlc.arg('used_at')
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: DeleteFileStoreConnection :execrows
DELETE FROM file_store_connection WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: InsertFileStoreAuthorization :exec
INSERT INTO file_store_authorization (id, tenant_id, account_id, kind, server, poll_endpoint, state_hash, secret_enc, secret_key_id, expires_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('kind'), sqlc.arg('server'),
        sqlc.arg('poll_endpoint'), sqlc.arg('state_hash'), sqlc.arg('secret_enc'), sqlc.arg('secret_key_id'), sqlc.arg('expires_at'));

-- name: FindFileStoreAuthorization :one
SELECT id, account_id, kind, server, poll_endpoint, secret_enc, secret_key_id, expires_at
FROM file_store_authorization
WHERE tenant_id = current_tenant_id() AND state_hash = sqlc.arg('state_hash');

-- name: DeleteFileStoreAuthorization :exec
DELETE FROM file_store_authorization WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: DeleteExpiredFileStoreAuthorizations :execrows
DELETE FROM file_store_authorization WHERE tenant_id = current_tenant_id() AND expires_at <= sqlc.arg('now');
