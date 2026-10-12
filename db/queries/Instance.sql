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

-- name: ResolveOperatorAccount :one
-- The account an address names inside one workspace, through the register's own narrow door
-- (migration 0109). An operator cannot list accounts across workspaces - `account` is behind row
-- level security - so an identifier is not something they can look up, and an address and a slug
-- are what a person knows. Answers one identifier or nothing.
SELECT resolve_operator_account(sqlc.arg('slug'), sqlc.arg('email'));

-- name: AddOperator :one
-- The workspace comes from the account rather than from the caller: false is "no such account", and
-- a pair that could disagree would be a pair somebody eventually gets wrong.
SELECT add_operator(sqlc.arg('account_id'), sqlc.narg('added_by'));

-- name: DropOperator :one
-- False where the register would have been emptied: the last operator cannot remove themselves.
SELECT drop_operator(sqlc.arg('account_id'));

-- name: OperatorRoster :many
-- The register with names, for the control plane's own screen (migration 0135): the account's name,
-- address or sign-in name and kind, its workspace, and who added it. No row unless the caller is an
-- operator, and never an account the register does not hold.
-- The casts are for the generator: it cannot see into the function's OUT table; empty rather than
-- NULL for the text columns that may be absent (`AdminTenants`' own note).
SELECT tenant_id::uuid AS tenant_id, account_id::uuid AS account_id,
       added_at::timestamptz AS added_at, added_by::uuid AS added_by,
       display_name::text AS display_name,
       coalesce(email, '')::text AS email, coalesce(sign_in_name, '')::text AS sign_in_name,
       kind::text AS kind, workspace_slug::text AS workspace_slug,
       workspace_name::text AS workspace_name, coalesce(added_by_name, '')::text AS added_by_name
FROM operator_roster(sqlc.arg('caller'));

-- name: EnlistOperator :one
-- Adds an active account, and the caller too where the register was empty - the caller was an
-- operator only because it was. Answers ADDED, ADDED_WITH_CALLER, PRESENT, NOT_ACTIVE or UNKNOWN.
SELECT enlist_operator(sqlc.arg('account_id'), sqlc.narg('added_by'))::text;

-- name: DismissOperator :one
-- Removes one, never the last. Answers REMOVED, LAST or ABSENT.
SELECT dismiss_operator(sqlc.arg('account_id'))::text;

-- name: InstanceCensus :one
-- The installation at a glance: counts, states and limits, never rows. Through the function
-- rather than against the tables: `account` is behind row level security and FORCE, so the
-- application role cannot count across workspaces at all - and narrow by construction is what makes
-- that exception acceptable (migration 0105).
-- The casts are for the generator: it cannot see into the function's OUT table
-- (`AdminTenants`' own note).
SELECT workspaces_active::bigint, workspaces_suspended::bigint,
       workspaces_pending_deletion::bigint,
       accounts_active::bigint, accounts_total::bigint,
       workspaces_service_only::bigint
FROM instance_census();

-- name: ResolveSingleOperatorAccount :one
-- A bare address in `HUBTASK_OPERATORS` on a single-mode installation: its one workspace's account,
-- or nothing (migration 0128).
SELECT resolve_single_operator_account(sqlc.arg('email'));

-- name: OperatorRegisterState :one
-- Workspaces and operators, counted as `is_operator` counts, for the health report's warning about
-- an installation with several workspaces and nobody registered to run it (migration 0128).
SELECT workspaces::bigint, operators::bigint FROM operator_register_state();

-- name: InstallationUnset :one
-- No workspace and nobody in the register: the state in which a setup code is issued and the setup
-- card is offered (migration 0128).
SELECT installation_unset();

-- name: IssueSetupCode :one
-- Lands only on an empty installation; false otherwise.
SELECT issue_setup_code(sqlc.arg('id'), sqlc.arg('code_hash'), sqlc.arg('issued_at'), sqlc.arg('expires_at'));

-- name: ClaimSetupCode :one
-- Inside the setup's transaction: ALREADY_DONE, WRONG, EXPIRED or CLAIMED (migration 0128).
SELECT claim_setup_code(sqlc.arg('code_hash'), sqlc.arg('now'))::text;

-- name: InstanceQuotaDefaults :many
-- What the installation set as the default ceiling for each quota, which every workspace that set
-- nothing of its own falls back to (ADR-0070 §2, the concept's §6.7).
--
-- `instance_setting` carries no row level security — that is its documented exception — so this
-- read works inside a tenant's own transaction, where the quota guard runs.
SELECT key, value FROM instance_setting WHERE key LIKE 'quota.%';
