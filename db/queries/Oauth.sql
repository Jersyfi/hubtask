-- The OAuth2 provider (H-05). The tenant is never a parameter: row level security bounds every
-- statement to the tenant of the running transaction (ADR-0010).

-- name: InsertOauthClient :exec
-- The secret's hash is computed in the adapter, the pepper's home (security.md §8); NULL for a
-- public client, whose whole authentication is PKCE.
INSERT INTO oauth_client
  (id, tenant_id, name, confidential, secret_hash, redirect_uris, created_at, created_by)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('name'), sqlc.arg('confidential'),
  sqlc.narg('secret_hash'), sqlc.arg('redirect_uris'), sqlc.arg('created_at'),
  sqlc.narg('created_by')
);

-- name: ListOauthClients :many
SELECT id, name, confidential, redirect_uris, created_at
FROM oauth_client
ORDER BY created_at DESC, id DESC;

-- name: FindOauthClient :one
-- The exchange's read: everything judging a token request needs, the stored hash included -
-- compared in the adapter, never answered upwards.
SELECT id, name, confidential, secret_hash, redirect_uris, created_at
FROM oauth_client
WHERE id = sqlc.arg('id');

-- name: DeleteOauthClient :execrows
-- The grants go by cascade, and the sessions those grants leashed go with them: a door is
-- removed with every key that opened it.
DELETE FROM oauth_client WHERE id = sqlc.arg('id');

-- name: UpsertOauthGrant :one
-- One live grant per person and app: a fresh consent is the newest answer and replaces the
-- scopes, rather than accumulating every set ever agreed to.
INSERT INTO oauth_grant (id, tenant_id, account_id, client_id, scopes, created_at)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('client_id'),
  sqlc.arg('scopes'), sqlc.arg('created_at')
)
ON CONFLICT (account_id, client_id) WHERE revoked_at IS NULL
DO UPDATE SET scopes = EXCLUDED.scopes
RETURNING id;

-- name: FindOauthGrant :one
SELECT id, account_id, client_id, scopes, created_at, revoked_at
FROM oauth_grant
WHERE id = sqlc.arg('id');

-- name: ListOauthGrants :many
-- One's own, newest first, with the client's name and when a session under the grant last
-- acted - computed from the sessions rather than written back, so the listing is accurate
-- without the hot path paying a second write.
SELECT g.id, g.client_id, c.name AS client_name, g.scopes, g.created_at,
       max(s.last_seen_at) AS last_used_at
FROM oauth_grant g
JOIN oauth_client c ON c.id = g.client_id
LEFT JOIN session s ON s.grant_id = g.id
WHERE g.account_id = sqlc.arg('account_id') AND g.revoked_at IS NULL
GROUP BY g.id, c.name
ORDER BY g.created_at DESC, g.id DESC;

-- name: RevokeOauthGrant :execrows
-- Bounded to the owner in the same statement that writes; only the first withdrawal writes.
UPDATE oauth_grant SET revoked_at = sqlc.arg('revoked_at')
WHERE id = sqlc.arg('id') AND account_id = sqlc.arg('account_id') AND revoked_at IS NULL;

-- name: RevokeGrantSessions :execrows
-- The leash pulled: every session the grant issued refuses on its next request, the way refresh
-- reuse would end them.
UPDATE session SET revoked_at = sqlc.arg('revoked_at')
WHERE grant_id = sqlc.arg('grant_id') AND revoked_at IS NULL;

-- name: InsertOauthCode :exec
INSERT INTO oauth_code
  (id, tenant_id, client_id, account_id, grant_id, code_hash, code_challenge, redirect_uri,
   created_at, expires_at)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('client_id'), sqlc.arg('account_id'),
  sqlc.arg('grant_id'), sqlc.arg('code_hash'), sqlc.arg('code_challenge'),
  sqlc.arg('redirect_uri'), sqlc.arg('created_at'), sqlc.arg('expires_at')
);

-- name: ConsumeOauthCode :one
-- Judged and burned in one statement: unexpired, unconsumed, or nothing - a replayed code
-- matches no row, whoever races whom.
UPDATE oauth_code SET consumed_at = sqlc.arg('now')
WHERE code_hash = sqlc.arg('code_hash')
  AND consumed_at IS NULL
  AND expires_at > sqlc.arg('now')
RETURNING id, client_id, account_id, grant_id, code_challenge, redirect_uri;

-- name: DeleteExpiredOauthCodes :execrows
-- Hygiene in the session sweep's pass, auth_pending's reasoning: a code lives minutes.
DELETE FROM oauth_code
WHERE id IN (
  SELECT id FROM oauth_code AS expired
  WHERE expired.expires_at < sqlc.arg('cutoff')
  LIMIT sqlc.arg('batch')
);
