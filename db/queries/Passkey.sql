-- Passkeys and security keys (identity.md §23). Row security keeps every statement in the
-- transaction's workspace; the account is named because a person reads and writes only their own.

-- name: ListPasskeys :many
SELECT id, tenant_id, account_id, credential_id, public_key, algorithm, sign_count, user_handle,
       backup_eligible, backed_up, transports, name, created_at, last_used_at, rp_id
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
  backup_eligible, backed_up, transports, name, created_at, rp_id
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('credential_id'),
  sqlc.arg('public_key'), sqlc.arg('algorithm'), sqlc.arg('sign_count'), sqlc.arg('user_handle'),
  sqlc.arg('backup_eligible'), sqlc.arg('backed_up'), sqlc.arg('transports')::text[],
  sqlc.arg('name'), sqlc.arg('created_at'), sqlc.narg('rp_id')
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

-- name: RenamePasskey :one
-- Only the account's own: another's, another workspace's or a removed one matches nothing.
UPDATE webauthn_credential SET name = sqlc.arg('name')
WHERE id = sqlc.arg('id') AND account_id = sqlc.arg('account_id')
RETURNING id, tenant_id, account_id, credential_id, public_key, algorithm, sign_count, user_handle,
          backup_eligible, backed_up, transports, name, created_at, last_used_at, rp_id;

-- name: RemovePasskey :execrows
-- Only the account's own; it signs and proves nothing from the moment this commits.
DELETE FROM webauthn_credential WHERE id = sqlc.arg('id') AND account_id = sqlc.arg('account_id');

-- name: InsertPasskeySignInChallenge :exec
-- A sign-in's challenge names nobody yet (migration 0174).
INSERT INTO auth_pending (id, tenant_id, token_hash, purpose, user_agent, ip_class, created_at, expires_at)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('token_hash'), 'PASSKEY_SIGN_IN',
  sqlc.narg('user_agent'), sqlc.narg('ip_class'), sqlc.arg('created_at'), sqlc.arg('expires_at')
);

-- name: ConsumePasskeySignInChallenge :execrows
-- One statement, so two completions of one challenge let exactly one through.
UPDATE auth_pending SET consumed_at = sqlc.arg('now')
WHERE token_hash = sqlc.arg('token_hash') AND purpose = 'PASSKEY_SIGN_IN'
  AND consumed_at IS NULL AND expires_at > sqlc.arg('now');

-- name: FindPasskeyHolder :one
-- The credential a browser named and everything the session will need of its holder, in one round
-- trip - the password door's read, keyed on the credential instead of an identifier.
SELECT w.id, w.tenant_id, w.account_id, w.credential_id, w.public_key, w.algorithm, w.sign_count,
       w.user_handle, w.backup_eligible, w.backed_up, w.transports, w.name, w.created_at,
       w.last_used_at, w.rp_id,
       a.kind, a.email, a.sign_in_name, a.display_name, a.status,
       a.locale AS account_locale, a.time_zone AS account_time_zone, a.start_password_at,
       n.default_locale, n.default_time_zone, n.slug AS tenant_slug, n.status::text AS tenant_status
FROM webauthn_credential w
JOIN account a ON a.tenant_id = w.tenant_id AND a.id = w.account_id
JOIN tenant n ON n.id = w.tenant_id
WHERE w.credential_id = sqlc.arg('credential_id') AND a.deleted_at IS NULL;

-- name: MarkPasskeyUsed :execrows
-- The counter only goes up: a counter not above a non-zero stored one is refused here, in the
-- statement, so two sign-ins with one credential cannot both pass a comparison; a synchronised
-- passkey's zero passes and never lowers the stored value (identity.md §23.3).
UPDATE webauthn_credential
SET sign_count = GREATEST(sign_count, sqlc.arg('sign_count')::bigint),
    last_used_at = sqlc.arg('used_at'), backed_up = sqlc.arg('backed_up')
WHERE id = sqlc.arg('id')
  AND (sqlc.arg('sign_count')::bigint = 0 OR sign_count = 0 OR sqlc.arg('sign_count')::bigint > sign_count);
