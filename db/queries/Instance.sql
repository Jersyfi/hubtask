-- name: ReadInstanceSettings :many
-- The installation's own level (ADR-0070 §2).
--
-- The whole set in one read, because it is small by construction - eighteen switches and four
-- links - and because the resolver needs all of it to answer one password screen. No tenant
-- predicate: the table deliberately carries no policy, and the transaction reading it has no
-- tenant at all.
SELECT key, value, lock_origin, updated_by, updated_at
FROM instance_setting
ORDER BY key;

-- name: PutInstanceSetting :exec
-- One switch, written or overwritten.
INSERT INTO instance_setting (key, value, lock_origin, updated_by, updated_at)
VALUES (
  sqlc.arg('key'), sqlc.arg('value'), sqlc.arg('lock_origin'),
  sqlc.narg('updated_by'), sqlc.arg('updated_at')
)
ON CONFLICT (key) DO UPDATE
  SET value = excluded.value,
      lock_origin = excluded.lock_origin,
      updated_by = excluded.updated_by,
      updated_at = excluded.updated_at;

-- name: DeleteInstanceSettingsExcept :exec
-- What the write did not decide stops being decided.
--
-- In the same transaction as the writes, which is what makes `PUT` mean what it says: a switch the
-- operator cleared is gone rather than left standing at its old value. Bounded to the areas the
-- caller wrote, so that a future area of settings is not emptied by a write to this one.
DELETE FROM instance_setting
WHERE split_part(key, '.', 1) = ANY(sqlc.arg('areas')::text[])
  AND NOT (key = ANY(sqlc.arg('kept')::text[]));

-- name: IsOperator :one
-- The register's check, asked when `admin:tenants` is minted and when it is exercised (ADR-0070 §1).
-- Through the function rather than against the table: `operator` carries no policy and no grant, so
-- the four narrow doors are the only way to it.
SELECT is_operator(sqlc.arg('account_id'));

-- name: OperatorRegister :many
-- The listing, for the control plane's own screen.
SELECT * FROM operator_register();

-- name: AddOperator :one
-- The workspace comes from the account rather than from the caller: false is "no such account", and
-- a pair that could disagree would be a pair somebody eventually gets wrong.
SELECT add_operator(sqlc.arg('account_id'), sqlc.narg('added_by'));

-- name: DropOperator :one
-- False where the register would have been emptied: the last operator cannot remove themselves.
SELECT drop_operator(sqlc.arg('account_id'));

-- name: InstanceCensus :one
-- The installation at a glance (SI-17): counts, states and limits, never rows. Through the function
-- rather than against the tables: `account` is behind row level security and FORCE, so the
-- application role cannot count across workspaces at all - and narrow by construction is what makes
-- that exception acceptable (migration 0105).
-- The casts are for the generator: it cannot see into the function's OUT table
-- (`AdminTenants`' own note).
SELECT workspaces_active::bigint, workspaces_suspended::bigint,
       workspaces_pending_deletion::bigint,
       accounts_active::bigint, accounts_total::bigint
FROM instance_census();
