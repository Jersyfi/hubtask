-- Custom roles and where they are held (migration 0219, UC-ID-26). The tenant is the
-- transaction's in every statement (ADR-0010).

-- name: ListCustomRoles :many
SELECT id, tenant_id, key, name, permissions, created_at, updated_at, version
FROM custom_role
ORDER BY key;

-- name: FindCustomRole :one
SELECT id, tenant_id, key, name, permissions, created_at, updated_at, version
FROM custom_role WHERE key = sqlc.arg('key');

-- name: InsertCustomRole :exec
INSERT INTO custom_role (id, tenant_id, key, name, permissions, created_at, updated_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('key'), sqlc.arg('name'),
        sqlc.arg('permissions')::text[], sqlc.arg('at'), sqlc.arg('at'));

-- name: UpdateCustomRole :execrows
-- Held to the version the writer read, so two administrators changing one role at once do not
-- silently drop each other's permission.
UPDATE custom_role
SET name = sqlc.arg('name'), permissions = sqlc.arg('permissions')::text[],
    updated_at = sqlc.arg('at'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('version');

-- name: DeleteCustomRole :execrows
-- Its grants go with it by cascade: access, never user content.
DELETE FROM custom_role WHERE id = sqlc.arg('id');

-- name: CustomRolesAlongPath :many
-- What the account holds of custom roles that could apply to this path, directly or through one of
-- its groups: MembershipsAlongPath's counterpart, bounded by the path for the same reason.
SELECT g.scope_type, g.scope_id, r.key, r.permissions
FROM custom_role_grant g
JOIN custom_role r ON r.tenant_id = g.tenant_id AND r.id = g.role_id
WHERE (
    g.account_id = sqlc.arg('account_id')
    OR g.group_id IN (
      SELECT group_id FROM account_group_member WHERE account_id = sqlc.arg('account_id')
    )
  )
  AND (g.scope_type = 'TENANT' OR g.scope_id = ANY(sqlc.arg('scope_ids')::uuid[]));

-- name: GrantCustomRole :execrows
-- Writes nothing when the subject already holds the role at the scope (custom_role_grant_uq).
INSERT INTO custom_role_grant (id, tenant_id, account_id, group_id, scope_type, scope_id, role_id, created_at)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.narg('account_id'), sqlc.narg('group_id'),
  sqlc.arg('scope_type'), sqlc.narg('scope_id'), sqlc.arg('role_id'), sqlc.arg('at')
)
ON CONFLICT DO NOTHING;

-- name: FindCustomRoleGrant :one
SELECT g.id, g.tenant_id, g.account_id, g.group_id, g.scope_type, g.scope_id, g.role_id, r.key AS role_key
FROM custom_role_grant g
JOIN custom_role r ON r.tenant_id = g.tenant_id AND r.id = g.role_id
WHERE g.id = sqlc.arg('id');

-- name: RevokeCustomRoleGrant :execrows
DELETE FROM custom_role_grant WHERE id = sqlc.arg('id');

-- name: ListCustomRoleGrants :many
-- Every grant of custom roles in the workspace, for the people screen and *who may do what*.
-- Bounded by the caller's limit.
SELECT g.id, g.tenant_id, g.account_id, g.group_id, g.scope_type, g.scope_id, g.role_id, r.key AS role_key
FROM custom_role_grant g
JOIN custom_role r ON r.tenant_id = g.tenant_id AND r.id = g.role_id
ORDER BY r.key, g.scope_type, g.id
LIMIT sqlc.arg('max_rows');
