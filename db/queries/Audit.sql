-- The audit trail. The application role may only INSERT and SELECT here (db/schema.sql, grants),
-- so there is deliberately no UPDATE and no DELETE in this file - the retention job drops whole
-- partitions instead (audit.md §3).

-- name: LockAuditChain :exec
-- Serialises the chain per tenant for the rest of the transaction.
--
-- The sequence number has to be gapless and the previous hash has to be the one that is actually
-- committed before this entry, so two transactions writing for the same tenant cannot both read
-- the same tail. The lock is transaction-scoped: it is released by the commit or the rollback,
-- never left behind by a process that died. Per tenant rather than global, so one busy workspace
-- does not serialise the whole installation.
SELECT pg_advisory_xact_lock(hashtext('audit_log:' || current_tenant_id()::text));

-- name: LastAuditEntry :one
-- The tail of this tenant's chain: the sequence number to continue from and the hash to chain to.
-- No row means this is the first entry.
SELECT seq, hash
FROM audit_log
WHERE tenant_id = current_tenant_id()
ORDER BY occurred_at DESC, seq DESC
LIMIT 1;

-- name: InsertAuditEntry :exec
INSERT INTO audit_log (
  id, tenant_id, seq, occurred_at, action, outcome, severity,
  actor_type, actor_id, actor_label, on_behalf_of_id,
  target_type, target_id, target_label,
  context, changes, legal_basis, prev_hash, hash
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('seq'), sqlc.arg('occurred_at'),
  sqlc.arg('action'), sqlc.arg('outcome'), sqlc.arg('severity'),
  sqlc.arg('actor_type'), sqlc.narg('actor_id'), sqlc.narg('actor_label'),
  sqlc.narg('on_behalf_of_id'),
  sqlc.narg('target_type'), sqlc.narg('target_id'), sqlc.narg('target_label'),
  sqlc.arg('context'), sqlc.arg('changes'), sqlc.narg('legal_basis'),
  sqlc.narg('prev_hash'), sqlc.arg('hash')
);

-- name: ListAuditEntries :many
-- One page of the trail, newest first, with every filter audit.md §5 names.
--
-- The filters are parameters rather than a query assembled from strings, which is CLAUDE.md rule 9
-- with no exception for "it is only a filter": every one of them is a `narg` that is either NULL -
-- meaning the condition is not there - or a value the driver binds. A `WHERE` built by hand for the
-- three or four filters a caller happened to send is exactly the shape T-06 is about.
--
-- `starts_with` rather than LIKE for the action, because a caller's `%` would otherwise be a
-- wildcard: `action` is a dotted code and a prefix filter on `auth.` is the whole point, so the
-- prefix is compared as text rather than as a pattern.
--
-- The boundary is the pair (occurred_at, id): entries written in the same transaction share a
-- timestamp, so a cursor on the time alone would either skip one or return one forever. The pair
-- is audit_id_uq's columns, which is the index this reads through.
--
-- `tenant_id` is selected rather than taken from the transaction, unlike every other read in this
-- schema. The hash covers the value stored in the row, so a verifier that took the tenant from its
-- own context would be recomputing the digest over what it expected rather than over what is
-- written down (audit.md §3).
SELECT id, tenant_id, seq, occurred_at, action, outcome, severity,
       actor_type, actor_id, actor_label, on_behalf_of_id,
       target_type, target_id, target_label,
       context, changes, legal_basis, prev_hash, hash
FROM audit_log
WHERE tenant_id = current_tenant_id()
  AND (sqlc.narg('from_time')::timestamptz IS NULL OR occurred_at >= sqlc.narg('from_time')::timestamptz)
  AND (sqlc.narg('to_time')::timestamptz IS NULL OR occurred_at < sqlc.narg('to_time')::timestamptz)
  AND (sqlc.narg('action_prefix')::text IS NULL OR starts_with(action, sqlc.narg('action_prefix')::text))
  AND (sqlc.narg('actor_id')::uuid IS NULL OR actor_id = sqlc.narg('actor_id')::uuid)
  AND (sqlc.narg('target_type')::text IS NULL OR target_type = sqlc.narg('target_type')::text)
  AND (sqlc.narg('target_id')::uuid IS NULL OR target_id = sqlc.narg('target_id')::uuid)
  AND (sqlc.narg('outcome')::text IS NULL OR outcome = sqlc.narg('outcome')::text)
  AND (
    sqlc.narg('cursor_occurred_at')::timestamptz IS NULL
    OR (occurred_at, id) < (sqlc.narg('cursor_occurred_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY occurred_at DESC, id DESC
LIMIT sqlc.arg('page_size');
