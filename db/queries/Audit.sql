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
