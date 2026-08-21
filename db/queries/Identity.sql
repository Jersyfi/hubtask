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

-- ============================== Accounts ==============================
-- The tenant is never a parameter in this file: row level security bounds every statement to the
-- tenant of the running transaction, which is what makes an account of another tenant not found
-- rather than forbidden (ADR-0010, multi-tenancy.md §2).

-- name: FindAccount :one
SELECT id, kind, email, display_name, status, locale, time_zone, week_start
FROM account
WHERE id = sqlc.arg('id') AND deleted_at IS NULL;

-- name: FindAccountByEmail :one
-- Compared lower case, the way the uniqueness index does - two spellings of one address are two
-- accounts for one person otherwise (account_email_uq).
SELECT id, kind, email, display_name, status, locale, time_zone, week_start
FROM account
WHERE lower(email) = lower(sqlc.arg('email')) AND deleted_at IS NULL;

-- name: InsertAccount :exec
INSERT INTO account (id, tenant_id, kind, email, display_name, status, locale, time_zone, week_start)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('kind'), sqlc.narg('email'),
  sqlc.arg('display_name'), sqlc.arg('status'), sqlc.narg('locale'), sqlc.narg('time_zone'),
  sqlc.narg('week_start')
);

-- name: UpdateAccountPreferences :execrows
-- Three columns and no others. An update that could write any column is one that can write the
-- status by accident, and the status is what decides whether an account may act at all.
UPDATE account SET
  locale     = sqlc.narg('locale'),
  time_zone  = sqlc.narg('time_zone'),
  week_start = sqlc.narg('week_start'),
  updated_at = sqlc.arg('updated_at')
WHERE id = sqlc.arg('id') AND deleted_at IS NULL;

-- =============================== Groups ===============================

-- name: FindGroup :one
SELECT id, name, description, version FROM account_group WHERE id = sqlc.arg('id');

-- name: InsertGroup :exec
INSERT INTO account_group (id, tenant_id, name, description, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('name'), sqlc.narg('description'), 1);

-- name: UpdateGroup :execrows
-- Optimistic locking in the WHERE clause: the update matches nothing when somebody else has moved
-- the row on, and the caller learns that rather than overwriting them (api-guidelines.md).
UPDATE account_group SET
  name        = sqlc.arg('name'),
  description = sqlc.narg('description'),
  version     = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('expected_version');

-- name: DeleteGroup :execrows
-- The memberships the group granted go with it, and so do its member links - both by cascade
-- (db/migrations/0001_init.sql), which is one statement that cannot half-run rather than three
-- that can.
DELETE FROM account_group WHERE id = sqlc.arg('id');

-- name: AddGroupMember :exec
INSERT INTO account_group_member (tenant_id, group_id, account_id)
VALUES (current_tenant_id(), sqlc.arg('group_id'), sqlc.arg('account_id'))
ON CONFLICT DO NOTHING;

-- name: RemoveGroupMember :exec
DELETE FROM account_group_member
WHERE group_id = sqlc.arg('group_id') AND account_id = sqlc.arg('account_id');

-- name: GroupMembers :many
SELECT account_id FROM account_group_member WHERE group_id = sqlc.arg('group_id');

-- ============================ Memberships =============================

-- name: GrantMembership :exec
-- Idempotent by the same index that keeps a subject from holding one role twice at one scope: a
-- retried request is not a second grant.
INSERT INTO membership (id, tenant_id, account_id, group_id, scope_type, scope_id, role)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.narg('account_id'), sqlc.narg('group_id'),
  sqlc.arg('scope_type'), sqlc.narg('scope_id'), sqlc.arg('role')
)
ON CONFLICT DO NOTHING;

-- name: FindMembership :one
SELECT id, account_id, group_id, scope_type, scope_id, role
FROM membership WHERE id = sqlc.arg('id');

-- name: RevokeMembership :execrows
DELETE FROM membership WHERE id = sqlc.arg('id');
