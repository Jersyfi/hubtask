-- A person's stars (migration 0389, UC-WRK-37). The tenant is the transaction's in every
-- statement (ADR-0010); another workspace's list is never read.

-- name: FindAccountStars :one
SELECT stars, version FROM account_star
WHERE tenant_id = current_tenant_id() AND account_id = sqlc.arg('account_id');

-- name: InsertAccountStars :execrows
-- The first write of a list; a list another device wrote meanwhile makes it write nothing.
INSERT INTO account_star (tenant_id, account_id, stars, version, updated_at)
VALUES (current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('stars'), 1, sqlc.arg('at'))
ON CONFLICT (tenant_id, account_id) DO NOTHING;

-- name: UpdateAccountStars :execrows
-- Optimistic: zero rows is a list changed meanwhile.
UPDATE account_star SET stars = sqlc.arg('stars'), version = version + 1, updated_at = sqlc.arg('at')
WHERE tenant_id = current_tenant_id() AND account_id = sqlc.arg('account_id')
  AND version = sqlc.arg('expected_version');
