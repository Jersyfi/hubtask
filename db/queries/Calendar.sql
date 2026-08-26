-- The calendar feeds (D-08, api-guidelines.md §7).
--
-- The tenant is never a parameter: it comes from the transaction's own context through
-- current_tenant_id(), which is the value row level security compares against (ADR-0010). That
-- holds for the public route as well - the token names its tenant, the caller opens the
-- transaction as that tenant, and the lookup below is then an ordinary tenant-scoped query.
--
-- The token itself never appears here. What is stored is a hash the adapter computes with the
-- installation's pepper, under this token's own purpose label, so a stolen dump cannot be turned
-- back into a working feed URL and a hash from here cannot be replayed as a cursor or a personal
-- access token (security.md §5, §8).

-- name: InsertCalendarFeed :exec
INSERT INTO calendar_feed (id, tenant_id, account_id, view_id, token_hash, created_at)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('view_id'),
  sqlc.arg('token_hash'), sqlc.arg('created_at')
);

-- name: FindCalendarFeedByHash :one
-- The public route's whole lookup: one index seek on the unique hash.
--
-- A revoked feed is returned rather than filtered out. Whether a token still works is the domain's
-- question, and the answer a fetch gets is the same either way - what changes is that the
-- application layer can tell the two apart in its own reasoning without the query deciding for it.
SELECT id, tenant_id, account_id, view_id, created_at, revoked_at
FROM calendar_feed
WHERE token_hash = $1;

-- name: FindCalendarFeed :one
SELECT id, tenant_id, account_id, view_id, created_at, revoked_at
FROM calendar_feed
WHERE id = $1;

-- name: ListCalendarFeedsForAccount :many
-- One person's own feeds, newest first. The account is a parameter rather than implied, because
-- the row level security policy on this table is the tenant's and the "only mine" half is the
-- application layer's rule - stated here as the condition it is, and enforced there as well.
SELECT id, tenant_id, account_id, view_id, created_at, revoked_at
FROM calendar_feed
WHERE account_id = sqlc.arg('account_id')
ORDER BY created_at DESC, id DESC;

-- name: RevokeCalendarFeed :execrows
-- Revocation is a stamp rather than a delete: "that token was revoked on Tuesday" is a question
-- somebody asks after a laptop goes missing, and a deleted row answers it with silence.
--
-- The revoked_at IS NULL condition is what makes the moment the first one. A second revocation
-- changes no row, which is how the caller learns it was already revoked without reading first.
UPDATE calendar_feed
SET revoked_at = sqlc.arg('revoked_at')
WHERE id = sqlc.arg('id')
  AND revoked_at IS NULL;
