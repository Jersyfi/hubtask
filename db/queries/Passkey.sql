-- Passkeys and security keys (identity.md §23). Row security keeps every statement in the
-- transaction's workspace; the account is named because a person reads and writes only their own.

-- name: ListPasskeys :many
SELECT id, tenant_id, account_id, credential_id, public_key, algorithm, sign_count, user_handle,
       backup_eligible, backed_up, transports, name, created_at, last_used_at
FROM webauthn_credential
WHERE account_id = sqlc.arg('account_id')
ORDER BY created_at, id;

-- name: CountPasskeys :one
SELECT count(*) FROM webauthn_credential WHERE account_id = sqlc.arg('account_id');

-- name: LockAccountForPasskeys :one
-- The row lock the limit's count and insert run under.
SELECT id FROM account WHERE id = sqlc.arg('account_id') FOR UPDATE;

-- name: InsertPasskey :execrows
-- A credential identifier already held in this workspace writes nothing.
INSERT INTO webauthn_credential (
  id, tenant_id, account_id, credential_id, public_key, algorithm, sign_count, user_handle,
  backup_eligible, backed_up, transports, name, created_at
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('credential_id'),
  sqlc.arg('public_key'), sqlc.arg('algorithm'), sqlc.arg('sign_count'), sqlc.arg('user_handle'),
  sqlc.arg('backup_eligible'), sqlc.arg('backed_up'), sqlc.arg('transports')::text[],
  sqlc.arg('name'), sqlc.arg('created_at')
)
ON CONFLICT (tenant_id, credential_id) DO NOTHING;

-- name: SupersedePasskeyChallenges :exec
-- The newest start wins: an older challenge of the account is spent, so it finishes nothing.
UPDATE auth_pending SET consumed_at = sqlc.arg('now')
WHERE account_id = sqlc.arg('account_id') AND purpose = 'PASSKEY_REGISTER' AND consumed_at IS NULL;

-- name: InsertPasskeyChallenge :exec
INSERT INTO auth_pending (id, tenant_id, account_id, token_hash, purpose, created_at, expires_at,
                          session_id, user_handle)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('token_hash'),
  'PASSKEY_REGISTER', sqlc.arg('created_at'), sqlc.arg('expires_at'),
  sqlc.arg('session_id'), sqlc.arg('user_handle')
);

-- name: ConsumePasskeyChallenge :one
-- One statement, so two completions of one challenge let exactly one through.
UPDATE auth_pending SET consumed_at = sqlc.arg('now')
WHERE token_hash = sqlc.arg('token_hash') AND purpose = 'PASSKEY_REGISTER'
  AND session_id = sqlc.arg('session_id') AND account_id = sqlc.arg('account_id')
  AND consumed_at IS NULL AND expires_at > sqlc.arg('now')
RETURNING id, account_id, session_id, user_handle, created_at, expires_at;
