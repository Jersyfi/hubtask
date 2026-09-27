-- name: FindAccountForPassword :one
-- What setting or judging a password needs of the account (ADR-0068 §5).
SELECT
  a.id, a.tenant_id, a.kind, a.email, a.display_name, a.status,
  a.locale, a.time_zone, a.week_start,
  a.password_hash, a.password_set_at
FROM account a
WHERE a.id = $1 AND a.deleted_at IS NULL;

-- name: FindAccountByEmailForPassword :one
-- The same, by address, for the reset that must answer identically whether or not it found one.
SELECT
  a.id, a.tenant_id, a.kind, a.email, a.display_name, a.status,
  a.locale, a.time_zone, a.week_start,
  a.password_hash, a.password_set_at
FROM account a
WHERE lower(a.email) = lower(sqlc.arg('email')) AND a.deleted_at IS NULL;

-- name: SetAccountPassword :execrows
-- The hash and the moment in one statement, so no reader sees one without the other.
UPDATE account
   SET password_hash = sqlc.arg('password_hash'),
       password_set_at = sqlc.arg('at'),
       updated_at = sqlc.arg('at'),
       version = version + 1
 WHERE id = sqlc.arg('account_id') AND deleted_at IS NULL;

-- name: RecentPasswordHashes :many
-- The newest few, newest first. The depth is the policy's, passed in.
SELECT password_hash
FROM account_password_history
WHERE account_id = sqlc.arg('account_id')
ORDER BY set_at DESC
LIMIT sqlc.arg('depth');

-- name: AppendPasswordHistory :exec
INSERT INTO account_password_history (id, tenant_id, account_id, password_hash, set_at)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'),
  sqlc.arg('password_hash'), sqlc.arg('set_at')
);

-- name: TrimPasswordHistory :exec
-- Everything past the newest `keep`, dropped in the same transaction as the append - so the table
-- cannot grow past the policy even after the policy is lowered.
DELETE FROM account_password_history h
WHERE h.account_id = sqlc.arg('account_id')
  AND h.id NOT IN (
    SELECT k.id FROM account_password_history k
    WHERE k.account_id = sqlc.arg('account_id')
    ORDER BY k.set_at DESC
    LIMIT sqlc.arg('keep')
  );
