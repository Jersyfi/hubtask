-- A change of a workspace's address (multi-tenancy.md §3.1, ADR-0095 §3). Every statement is bounded
-- to the transaction's workspace by row level security or by current_tenant_id(); none takes a tenant.

-- name: ChangeTenantSlug :execrows
-- The short name moves with a subdomain; a name another workspace holds is the unique index's
-- refusal. Refused once the workspace is leaving. The moment starts an owner's cooldown (0210).
UPDATE tenant
SET slug = sqlc.arg('slug'), version = version + 1, updated_at = sqlc.arg('now'),
    address_changed_at = sqlc.arg('now')
WHERE id = current_tenant_id() AND deleted_at IS NULL AND purge_started_at IS NULL
  AND status <> 'PENDING_DELETION';

-- name: TenantAddressChangedAt :one
-- When the transaction's workspace last changed its address; NULL where it never did.
SELECT address_changed_at FROM tenant WHERE id = current_tenant_id();

-- name: LockTenantAddressChangedAt :one
-- The same, holding the row: two owners' changes cannot both pass the cooldown.
SELECT address_changed_at FROM tenant WHERE id = current_tenant_id() FOR UPDATE;

-- name: DropMainTenantHost :many
-- The old main row goes in the change's own transaction, so the old host resolves to nothing at once.
DELETE FROM tenant_host WHERE is_canonical RETURNING host;

-- name: RevokeWorkspaceSessions :execrows
-- Everybody signs in again. A session an OAuth grant issued is left: connected apps keep working.
UPDATE session SET revoked_at = sqlc.arg('revoked_at')
WHERE revoked_at IS NULL AND grant_id IS NULL;

-- name: PeopleToTellOfAddress :many
-- The people a change of address is told to: active and invited persons, never a service account.
-- Whether each has an address decides between the mail and the notice at sign-in.
SELECT id, (email IS NOT NULL AND email <> '')::boolean AS has_email
FROM account
WHERE kind = 'USER' AND deleted_at IS NULL AND status IN ('ACTIVE', 'INVITED')
ORDER BY id;

-- name: RecordAddressChangeNotice :exec
-- A second change before the member signed in keeps the address they last knew.
INSERT INTO address_change_notice (tenant_id, account_id, from_host, to_host, changed_at)
VALUES (current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('from_host'), sqlc.arg('to_host'),
        sqlc.arg('changed_at'))
ON CONFLICT (tenant_id, account_id)
DO UPDATE SET to_host = excluded.to_host, changed_at = excluded.changed_at;

-- name: AddressChangeNoticesOf :many
SELECT from_host, to_host, changed_at FROM address_change_notice
WHERE account_id = sqlc.arg('account_id') AND from_host <> to_host;

-- name: AcknowledgeAddressChangeNotice :execrows
DELETE FROM address_change_notice WHERE account_id = sqlc.arg('account_id');
