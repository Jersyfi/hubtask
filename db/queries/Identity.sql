-- name: FindAccessTokenByHash :one
SELECT
  t.id,
  t.tenant_id,
  t.account_id,
  t.scopes,
  t.expires_at,
  t.revoked_at,
  t.last_used_at,
  a.kind     AS account_kind,
  a.status   AS account_status,
  a.locale   AS account_locale,
  a.time_zone AS account_time_zone,
  n.default_locale,
  n.default_time_zone
FROM access_token t
JOIN account a ON a.id = t.account_id
JOIN tenant  n ON n.id = t.tenant_id
WHERE t.token_hash = $1
  AND a.deleted_at IS NULL;

-- name: TouchAccessToken :exec
UPDATE access_token SET last_used_at = $2 WHERE id = $1;
