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
  n.default_time_zone,
  n.slug         AS tenant_slug,
  n.status::text AS tenant_status,
  coalesce((n.settings #>> '{quotas,api_requests_per_minute}')::bigint, 0) AS token_rate_override
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

-- name: AdministratorsAlongPath :many
-- Who administers anywhere on this path, for the retention advance warning (R-1, G-12).
--
-- The mirror image of MembershipsAlongPath: that one asks what one account holds, this one asks
-- who holds something. The roles are named here rather than passed in, because "the people who can
-- answer a warning about work that is about to be deleted" is a property of the role matrix and
-- not a parameter a caller gets to choose - a caller that could name VIEWER would be a caller
-- warning everybody.
--
-- Through a group as well as directly, on MembershipsAlongPath's reasoning: a right held through a
-- group is not a lesser right, and an administrator who administers through one still administers.
-- Distinct, because somebody who holds the role at two levels is one person.
SELECT DISTINCT account_id FROM (
  SELECT m.account_id
  FROM membership m
  WHERE m.account_id IS NOT NULL
    -- Cast, because `role` is the enum `membership_role` and the argument is text: comparing the
    -- two directly is an operator PostgreSQL does not have, and the enum is what the column has
    -- been since the first migration.
    AND m.role::text = ANY(sqlc.arg('roles')::text[])
    AND (m.scope_type = 'TENANT' OR m.scope_id = ANY(sqlc.arg('scope_ids')::uuid[]))
  UNION
  SELECT g.account_id
  FROM membership m
  JOIN account_group_member g ON g.group_id = m.group_id
  WHERE m.group_id IS NOT NULL
    AND m.role::text = ANY(sqlc.arg('roles')::text[])
    AND (m.scope_type = 'TENANT' OR m.scope_id = ANY(sqlc.arg('scope_ids')::uuid[]))
) AS holders;

-- name: SharedItemsInCollection :many
-- The entries inside one collection that the account holds a membership on directly, or through
-- one of its groups.
--
-- What "shared with me" means (domain-model.md §3.2, C-04): a membership at ITEM scope reaches
-- that entry and nothing else, so the answer to "which of this collection's entries may I see"
-- is the list of those scope identifiers. It is asked only when the account holds no role on the
-- collection itself - the ordinary case answers the whole level in one check and never runs this.
--
-- The join is what bounds it: without it the query would return every entry shared with the
-- account anywhere in the tenant, and the caller asked about one collection.
SELECT m.scope_id
FROM membership m
JOIN work_item wi ON wi.id = m.scope_id
WHERE m.scope_type = 'ITEM'
  AND wi.collection_id = sqlc.arg('collection_id')
  AND (
    m.account_id = sqlc.arg('account_id')
    OR m.group_id IN (
      SELECT group_id FROM account_group_member WHERE account_id = sqlc.arg('account_id')
    )
  );

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

-- name: RestrictedAccounts :many
-- Which of the accounts named may not be processed automatically (Art. 18, data-protection.md §4).
--
-- Anonymised as well as restricted: an account whose person has been erased is not a candidate for
-- anything either, and the caller asks one question - "may this system decide something about
-- them" - rather than two.
SELECT id
FROM account
WHERE id = ANY(sqlc.arg('account_ids')::uuid[])
  AND status IN ('RESTRICTED', 'ANONYMIZED');

-- ========================== Personal access tokens ==========================
-- The tenant is never a parameter here either: row level security bounds every statement to the
-- tenant of the running transaction, which is what makes a token of another workspace invisible
-- rather than forbidden (ADR-0010).

-- name: InsertAccessToken :exec
-- The hash is computed in the adapter, because the pepper is a secret of that layer and the
-- application must never hold a value it could store by mistake (security.md §8).
INSERT INTO access_token
    (id, tenant_id, account_id, name, token_hash, token_prefix, scopes, expires_at, created_at)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('name'),
  sqlc.arg('token_hash'), sqlc.arg('token_prefix'), sqlc.arg('scopes'),
  sqlc.arg('expires_at'), sqlc.arg('created_at')
);

-- name: FindAccessToken :one
SELECT id, tenant_id, account_id, name, scopes, expires_at, last_used_at, revoked_at, created_at
FROM access_token
WHERE id = sqlc.arg('id');

-- name: AccessTokensForAccount :many
-- Newest first, which is the order somebody reads their own credentials in. Bounded by the
-- account rather than by a page: a person holds a handful, and the index answers both the filter
-- and the sort (access_token_account_idx).
SELECT id, tenant_id, account_id, name, scopes, expires_at, last_used_at, revoked_at, created_at
FROM access_token
WHERE account_id = sqlc.arg('account_id')
ORDER BY created_at DESC, id DESC;

-- name: RevokeAccessToken :execrows
-- Only the first withdrawal writes. A second call matches nothing, which is how the use case
-- tells "revoked just now" from "already revoked" without reading the row again - and what keeps
-- the moment it was first pulled from being overwritten.
UPDATE access_token SET revoked_at = sqlc.arg('revoked_at')
WHERE id = sqlc.arg('id') AND revoked_at IS NULL;

-- name: AccountsOfKind :many
-- The workspace's service accounts, newest first by identifier - UUIDv7 is time-ordered, so the
-- primary key is the creation order and needs no second column to sort on.
--
-- Bounded by the kind rather than by a page: an installation has a handful of integrations, and a
-- cursor over a handful is machinery nobody reads.
SELECT id, kind, email, display_name, status, locale, time_zone, week_start
FROM account
WHERE kind = sqlc.arg('kind') AND deleted_at IS NULL
ORDER BY id DESC;
