-- name: FindAccessTokenByHash :one
SELECT
  t.id,
  t.tenant_id,
  t.account_id,
  t.scopes,
  t.expires_at,
  t.revoked_at,
  t.last_used_at,
  a.display_name AS account_display_name,
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

-- name: MembershipsAlongPath :many
-- What the account holds that could apply to this path, directly or through one of its groups.
--
-- Bounded by the path rather than reading everything the account holds: a permission check runs
-- on every write, and an account in a large tenant may hold hundreds of memberships. It may be
-- generous - a tenant-wide membership always applies, and the resolution ignores whatever is not
-- on the path - but it must not be unbounded.
--
-- Whether the right is held directly or through a group is not distinguished in the result. The
-- question is what the account may do, and a right held through a group is not a lesser right.
SELECT m.scope_type, m.scope_id, m.role
FROM membership m
WHERE (
    m.account_id = sqlc.arg('account_id')
    OR m.group_id IN (
      SELECT group_id FROM account_group_member WHERE account_id = sqlc.arg('account_id')
    )
  )
  AND (m.scope_type = 'TENANT' OR m.scope_id = ANY(sqlc.arg('scope_ids')::uuid[]));
