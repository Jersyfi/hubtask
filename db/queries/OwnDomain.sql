-- A workspace's own domain (ADR-0095 §4, §5; migration 0211). The workspace's own statements are
-- bounded by row level security; the ones across workspaces go through the SECURITY DEFINER doors.

-- name: FindOwnDomain :one
SELECT host, state, verification, is_canonical, created_at, state_since, checked_at,
       target_found, proof_found, (certificate_asked_at IS NOT NULL)::boolean AS certificate_asked,
       fell_back_at
FROM tenant_host WHERE kind = 'OWN_DOMAIN';

-- name: InsertOwnDomain :exec
INSERT INTO tenant_host
  (tenant_id, host, kind, state, verification, is_canonical, created_at, state_since)
VALUES (current_tenant_id(), sqlc.arg('host'), 'OWN_DOMAIN', 'PENDING', sqlc.arg('verification'),
        false, sqlc.arg('now'), sqlc.arg('now'));

-- name: DeleteOwnDomain :one
-- The row goes, so the host answers like one no workspace has at once.
DELETE FROM tenant_host WHERE kind = 'OWN_DOMAIN' RETURNING host, is_canonical;

-- name: RecordOwnDomainLook :execrows
-- One look's outcome, guarded on the state it was taken from: a block, a removal or the first
-- request between the look and the write wins, and the next pass looks again.
UPDATE tenant_host
SET state = sqlc.arg('next_state')::text,
    state_since = CASE WHEN state <> sqlc.arg('next_state')::text THEN sqlc.arg('now')::timestamptz ELSE state_since END,
    verified_at = CASE WHEN sqlc.arg('next_state')::text IN ('VERIFIED', 'ACTIVE') AND verified_at IS NULL
                       THEN sqlc.arg('now')::timestamptz ELSE verified_at END,
    checked_at = sqlc.arg('now')::timestamptz,
    target_found = sqlc.arg('target_found'),
    proof_found = sqlc.arg('proof_found')
WHERE kind = 'OWN_DOMAIN' AND host = sqlc.arg('host') AND state = sqlc.arg('from_state')::text;

-- name: OwnDomainIsBlocked :one
SELECT own_domain_blocked(sqlc.arg('host')::text)::boolean AS blocked;

-- name: BlockInView :one
-- The block the workspace's owner still sees, the latest where there are several.
SELECT host, reason, blocked_at, was_main FROM blocked_domain
WHERE tenant_id = current_tenant_id() AND dismissed_at IS NULL
ORDER BY blocked_at DESC LIMIT 1;

-- name: DismissBlocks :execrows
UPDATE blocked_domain SET dismissed_at = sqlc.arg('now')
WHERE tenant_id = current_tenant_id() AND dismissed_at IS NULL;

-- name: InsertBlockedDomain :exec
INSERT INTO blocked_domain (host, tenant_id, reason, blocked_at, was_main)
VALUES (sqlc.arg('host'), current_tenant_id(), sqlc.arg('reason'), sqlc.arg('blocked_at'), sqlc.arg('was_main'));

-- name: OwnDomainsDue :many
SELECT tenant_id::uuid AS tenant_id, host::text AS host, verification::text AS verification,
       state::text AS state
FROM own_domains_due(sqlc.arg('pending_before')::timestamptz, sqlc.arg('serving_before')::timestamptz,
                     sqlc.arg('most')::integer);

-- name: AdminOwnDomains :many
SELECT host::text AS host, coalesce(tenant_id::text, '')::text AS tenant_id,
       tenant_name::text AS tenant_name, state::text AS state, since::timestamptz AS since,
       is_main::boolean AS is_main, coalesce(blocked_reason, '')::text AS blocked_reason,
       fell_back_at::timestamptz AS fell_back_at, coalesce(address_host, '')::text AS address_host
FROM admin_own_domains();

-- name: OwnDomainCertificate :one
SELECT own_domain_certificate(sqlc.arg('host')::text, sqlc.arg('asked_at')::timestamptz)::boolean AS allowed;

-- name: ResolveServingHost :one
SELECT tenant_id::uuid AS tenant_id, awaits_request::boolean AS awaits_request
FROM resolve_serving_host(sqlc.arg('host')::text);

-- name: OwnDomainServed :one
SELECT own_domain_served(sqlc.arg('host')::text, sqlc.arg('served_at')::timestamptz)::boolean AS moved;

-- The main address moves between the own domain and the workspace's address row (migration 0212).
-- Two statements, never one swap: the one-main index is checked row by row.

-- name: ClearMainHost :many
UPDATE tenant_host SET is_canonical = false WHERE is_canonical RETURNING host;

-- name: MakeOwnDomainMain :execrows
-- Only an active own domain becomes main; making it main clears the warning of a fall-back.
UPDATE tenant_host SET is_canonical = true, fell_back_at = NULL
WHERE kind = 'OWN_DOMAIN' AND state = 'ACTIVE';

-- name: MakeAddressMain :one
-- The workspace's subdomain or assigned host, the newest where an old base URL left several.
UPDATE tenant_host SET is_canonical = true
WHERE kind = 'ADDRESS' AND host = (
  SELECT a.host FROM tenant_host a WHERE a.kind = 'ADDRESS' AND a.state = 'ACTIVE'
  ORDER BY a.created_at DESC LIMIT 1)
RETURNING host;

-- name: MarkOwnDomainFellBack :exec
UPDATE tenant_host SET fell_back_at = sqlc.arg('now') WHERE kind = 'OWN_DOMAIN';

-- name: AddressHost :one
-- The workspace's own address beside an own domain: the way back.
SELECT host FROM tenant_host WHERE kind = 'ADDRESS' AND state = 'ACTIVE'
ORDER BY is_canonical DESC, created_at DESC LIMIT 1;

-- name: BrowserHost :one
SELECT coalesce(browser_host(sqlc.arg('host')::text), '')::text AS host;
