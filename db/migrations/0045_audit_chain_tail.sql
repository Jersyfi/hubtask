-- +goose Up
-- The index the chain's tail is read through, and the reason it now needs one.
--
-- `LastAuditEntry` used to order by `occurred_at DESC, seq DESC`, and that is a chain that breaks
-- under concurrency. Each caller takes its own `Clock.Now()` *before* it queues for the per-tenant
-- advisory lock, so the entry with the newest timestamp is not always the one with the highest
-- sequence number. A transaction that read such a tail continued from a sequence number that was
-- already taken; the unique index could not stop it, because a partitioned table's unique index has
-- to carry the partition key, and `(tenant_id, occurred_at, seq)` lets one `seq` appear twice under
-- two timestamps. What came out was duplicated sequence numbers, a chain that no longer verified,
-- and an `audit.chain_broken` entry reporting tampering that never happened.
--
-- The query is ordered by `seq` alone now, which is what the chain is defined over. That ordering
-- has no index behind it - every other index on this table leads with `occurred_at` - so without
-- this one the tail read becomes a scan of every partition on the busiest write path there is.
--
-- On the parent, so that every partition gets it, including the ones `ensure_audit_partition`
-- creates from here on.
CREATE INDEX IF NOT EXISTS audit_seq_idx ON audit_log (tenant_id, seq DESC);

-- +goose Down
DROP INDEX IF EXISTS audit_seq_idx;
