-- The relying-party surface (H-04): the workspace's provider, and the flows of one sign-in.
--
-- Every statement here runs inside the transaction wrapper that sets `app.tenant_id`, so the
-- policy underneath answers "which workspace" - `current_tenant_id()` is written on insert
-- rather than passed in, the way every tenant-scoped insert in this schema is.

-- name: UpsertIdentityProvider :one
-- Set whole, not patched: a provider half-changed is a provider nobody can reason about. The
-- version rises on every write, so a concurrent second configuration is visible as a conflict.
INSERT INTO identity_provider
  (tenant_id, issuer, client_id, client_secret_enc, client_secret_key_id,
   allowed_email_domains, enabled, created_at)
VALUES (
  current_tenant_id(), sqlc.arg('issuer'), sqlc.arg('client_id'),
  sqlc.arg('client_secret_enc'), sqlc.arg('client_secret_key_id'),
  sqlc.arg('allowed_email_domains'), sqlc.arg('enabled'), sqlc.arg('now')
)
ON CONFLICT (tenant_id) DO UPDATE SET
  issuer                = excluded.issuer,
  client_id             = excluded.client_id,
  client_secret_enc     = excluded.client_secret_enc,
  client_secret_key_id  = excluded.client_secret_key_id,
  allowed_email_domains = excluded.allowed_email_domains,
  enabled               = excluded.enabled,
  updated_at            = sqlc.arg('now'),
  version               = identity_provider.version + 1
RETURNING issuer, client_id, allowed_email_domains, enabled, created_at, updated_at, version;

-- name: FindIdentityProvider :one
-- What a reader is allowed to see: never the sealed secret. The one caller that needs it asks
-- for it by name below, so a read cannot spill it by accident.
SELECT issuer, client_id, allowed_email_domains, enabled, created_at, updated_at, version
FROM identity_provider;

-- name: FindIdentityProviderSecret :one
-- The token exchange's own read, separate from the one above so that opening the envelope is a
-- deliberate call and not a field that happens to be in a struct somebody logged.
SELECT issuer, client_id, client_secret_enc, client_secret_key_id, allowed_email_domains, enabled
FROM identity_provider;

-- name: DeleteIdentityProvider :execrows
DELETE FROM identity_provider;

-- name: InsertOidcFlow :exec
INSERT INTO oidc_flow
  (id, tenant_id, state_hash, code_verifier, nonce, created_at, expires_at)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('state_hash'), sqlc.arg('code_verifier'),
  sqlc.arg('nonce'), sqlc.arg('created_at'), sqlc.arg('expires_at')
);

-- name: ConsumeOidcFlow :one
-- Judged and burned in one statement, ConsumeOauthCode's discipline: unexpired and unconsumed,
-- or nothing at all - so a state presented twice matches no row whoever races whom.
UPDATE oidc_flow SET consumed_at = sqlc.arg('now')
WHERE state_hash = sqlc.arg('state_hash')
  AND consumed_at IS NULL
  AND expires_at > sqlc.arg('now')
RETURNING id, code_verifier, nonce;

-- name: DeleteExpiredOidcFlows :execrows
-- Hygiene in the session sweep's pass, the expired authorization codes' reasoning: a flow lives
-- minutes, and what is left of it after that is a row nobody will ever present again.
DELETE FROM oidc_flow
WHERE id IN (
  SELECT id FROM oidc_flow AS expired
  WHERE expired.expires_at < sqlc.arg('cutoff')
  LIMIT sqlc.arg('batch')
);

-- name: FindAccountByExternalSubject :one
-- The subject the provider vouched for, under the unique index that makes it one account per
-- workspace. Deleted accounts are excluded: an arriving subject whose account was deleted is a
-- first arrival, not a resurrection.
SELECT id, tenant_id, kind, email, display_name, status, locale, time_zone, week_start
FROM account
WHERE external_subject = sqlc.arg('external_subject') AND deleted_at IS NULL;

-- name: LinkAccountExternalSubject :execrows
-- Writes the subject onto an account that has none. The `IS NULL` is what makes this safe to
-- race: a second sign-in that got there first leaves nothing for this one to overwrite, and an
-- account already bound to another subject is never quietly re-pointed.
UPDATE account
SET external_subject = sqlc.arg('external_subject'),
    updated_at       = sqlc.arg('now'),
    version          = version + 1
WHERE id = sqlc.arg('id') AND deleted_at IS NULL AND external_subject IS NULL;
