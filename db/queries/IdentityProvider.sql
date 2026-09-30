-- The relying-party surface (H-04, SI-10): the providers a workspace signs in through, and the
-- flows of one sign-in.
--
-- Every statement here runs inside the transaction wrapper that sets `app.tenant_id`, so the
-- policy underneath answers "which level" - `current_tenant_id()` is written on insert rather than
-- passed in, and in the installation's own scope it is NULL, which is exactly the row that belongs
-- to no workspace. One insert statement therefore serves both levels, and nothing has to decide
-- which one it is in.

-- name: ListIdentityProviders :many
-- The ways in that are in force here: this workspace's own rows and the installation's, which the
-- read policy admits together (migration 0103). The workspace's come first - its own choices sit
-- above the default it inherited - and the sealed secret is in none of it.
SELECT id, tenant_id, issuer, client_id, display_name, kind, provisioning, position,
  allowed_email_domains, allowed_directories, enabled, created_at, updated_at, version
FROM identity_provider
ORDER BY (tenant_id IS NULL), position, created_at, id;

-- name: FindIdentityProviderByID :one
-- What a reader is allowed to see: never the sealed secret. The one caller that needs it asks for
-- it by name below, so a read cannot spill it by accident.
SELECT id, tenant_id, issuer, client_id, display_name, kind, provisioning, position,
  allowed_email_domains, allowed_directories, enabled, created_at, updated_at, version
FROM identity_provider
WHERE id = sqlc.arg('id');

-- name: FindIdentityProviderSecret :one
-- The token exchange's own read, separate from the one above so that opening the envelope is a
-- deliberate call and not a field that happens to be in a struct somebody logged.
SELECT id, tenant_id, issuer, client_id, display_name, kind, provisioning, position,
  client_secret_enc, client_secret_key_id, allowed_email_domains, allowed_directories, enabled
FROM identity_provider
WHERE id = sqlc.arg('id');

-- name: CountIdentityProviders :one
-- What the bound is checked against before an insert. The workspace's own only: the installation's
-- rows are not its to be limited by.
SELECT count(*) FROM identity_provider WHERE tenant_id = current_tenant_id();

-- name: InsertIdentityProvider :one
-- `current_tenant_id()` rather than an argument, and it is what makes one statement serve both
-- levels: a workspace's transaction writes its own row, and the installation's scope - where the
-- setting is the empty string and the function therefore NULL - writes the row that belongs to
-- nobody. A caller cannot choose which, because there is nothing to pass.
INSERT INTO identity_provider
  (id, tenant_id, issuer, client_id, client_secret_enc, client_secret_key_id,
   display_name, kind, provisioning, position, allowed_email_domains, allowed_directories,
   enabled, created_at)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('issuer'), sqlc.arg('client_id'),
  sqlc.arg('client_secret_enc'), sqlc.arg('client_secret_key_id'),
  sqlc.arg('display_name'), sqlc.arg('kind'), sqlc.arg('provisioning'), sqlc.arg('position'),
  sqlc.arg('allowed_email_domains'), sqlc.arg('allowed_directories'),
  sqlc.arg('enabled'), sqlc.arg('now')
)
RETURNING id, tenant_id, issuer, client_id, display_name, kind, provisioning, position,
  allowed_email_domains, allowed_directories, enabled, created_at, updated_at, version;

-- name: UpdateIdentityProvider :one
-- Set whole, not patched: a provider half-changed is a provider nobody can reason about. The
-- secret is the one exception and the reason is that there is no way to read it back - a caller
-- that sent none means "keep the one that is sealed", and COALESCE is what says so in a single
-- statement rather than a read-then-write two sign-ins could interleave.
--
-- The version rises on every write, so a concurrent second configuration is visible as a conflict.
UPDATE identity_provider SET
  issuer                = sqlc.arg('issuer'),
  client_id             = sqlc.arg('client_id'),
  client_secret_enc     = coalesce(sqlc.narg('client_secret_enc'), client_secret_enc),
  client_secret_key_id  = coalesce(sqlc.narg('client_secret_key_id'), client_secret_key_id),
  display_name          = sqlc.arg('display_name'),
  kind                  = sqlc.arg('kind'),
  provisioning          = sqlc.arg('provisioning'),
  position              = sqlc.arg('position'),
  allowed_email_domains = sqlc.arg('allowed_email_domains'),
  allowed_directories   = sqlc.arg('allowed_directories'),
  enabled               = sqlc.arg('enabled'),
  updated_at            = sqlc.arg('now'),
  version               = version + 1
WHERE id = sqlc.arg('id')
RETURNING id, tenant_id, issuer, client_id, display_name, kind, provisioning, position,
  allowed_email_domains, allowed_directories, enabled, created_at, updated_at, version;

-- name: DeleteIdentityProvider :execrows
DELETE FROM identity_provider WHERE id = sqlc.arg('id');

-- name: InsertOidcFlow :exec
INSERT INTO oidc_flow
  (id, tenant_id, provider_id, state_hash, code_verifier, nonce, created_at, expires_at)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.narg('provider_id'), sqlc.arg('state_hash'),
  sqlc.arg('code_verifier'), sqlc.arg('nonce'), sqlc.arg('created_at'), sqlc.arg('expires_at')
);

-- name: ConsumeOidcFlow :one
-- Judged and burned in one statement, ConsumeOauthCode's discipline: unexpired and unconsumed,
-- or nothing at all - so a state presented twice matches no row whoever races whom.
UPDATE oidc_flow SET consumed_at = sqlc.arg('now')
WHERE state_hash = sqlc.arg('state_hash')
  AND consumed_at IS NULL
  AND expires_at > sqlc.arg('now')
RETURNING id, provider_id, code_verifier, nonce;

-- name: DeleteExpiredOidcFlows :execrows
-- Hygiene in the session sweep's pass, the expired authorization codes' reasoning: a flow lives
-- minutes, and what is left of it after that is a row nobody will ever present again.
DELETE FROM oidc_flow
WHERE id IN (
  SELECT id FROM oidc_flow AS expired
  WHERE expired.expires_at < sqlc.arg('cutoff')
  LIMIT sqlc.arg('batch')
);

-- name: FindAccountByProviderSubject :one
-- The subject the provider vouched for, under the unique index that makes it one account per
-- provider per workspace (migration 0103). Deleted accounts are excluded: an arriving subject whose
-- account was deleted is a first arrival, not a resurrection.
SELECT a.id, a.tenant_id, a.kind, a.email, a.display_name, a.status, a.locale, a.time_zone,
  a.week_start, a.celebrations, a.onboarding_completed_at
FROM account_identity link
JOIN account a ON a.tenant_id = link.tenant_id AND a.id = link.account_id
WHERE link.provider_id = sqlc.arg('provider_id')
  AND link.subject = sqlc.arg('subject')
  AND a.deleted_at IS NULL;

-- name: LinkAccountIdentity :execrows
-- Writes the link, and races safely: the unique index on (tenant, provider, subject) is what
-- refuses a subject already spoken for, and `DO NOTHING` is what makes a second sign-in that got
-- there first leave nothing for this one to overwrite. An account already bound to another subject
-- at this provider is never quietly re-pointed, because the primary key refuses that too.
INSERT INTO account_identity (tenant_id, account_id, provider_id, subject, linked_at)
SELECT a.tenant_id, a.id, sqlc.arg('provider_id'), sqlc.arg('subject'), sqlc.arg('now')
FROM account a
WHERE a.id = sqlc.arg('account_id') AND a.deleted_at IS NULL
ON CONFLICT DO NOTHING;

-- name: ListIdentityProviderSecrets :many
-- The re-seal's read (ADR-0045): every row of *this level* that holds a wrapping, so a rotation can
-- move each one under the key it named when it was read.
--
-- `IS NOT DISTINCT FROM` rather than `=`, and that is the whole difference: in a workspace's scope
-- it is the workspace's own rows, and in the installation's - where `current_tenant_id()` is NULL -
-- it is the rows that belong to no workspace. Which is what makes the installation's provider
-- re-sealable at all, by a pass that runs in that scope.
SELECT id, client_secret_enc, client_secret_key_id
FROM identity_provider
WHERE tenant_id IS NOT DISTINCT FROM current_tenant_id();

-- name: RewrapIdentityProviderSecret :execrows
-- A re-seal (ADR-0045): the wrapping moves, the configuration does not, so the version stays -
-- an operator rotating the installation's keys has not changed anybody's provider.
UPDATE identity_provider
SET client_secret_enc = sqlc.arg('client_secret_enc'), client_secret_key_id = sqlc.arg('client_secret_key_id')
WHERE id = sqlc.arg('id') AND client_secret_key_id = sqlc.arg('expected_key_id');
