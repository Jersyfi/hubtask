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
--
-- Ordered by `seq`, and only by `seq`. It used to read `occurred_at DESC, seq DESC`, and that is a
-- chain that breaks under concurrency: each caller takes its own `Clock.Now()` *before* it queues
-- for the advisory lock, so the entry with the newest timestamp is not always the one with the
-- highest sequence number. A transaction that read such a tail continued from a sequence number
-- that was already taken - the unique index cannot stop it, because a partitioned table's unique
-- index has to carry the partition key and `(tenant_id, occurred_at, seq)` lets one `seq` appear
-- twice under two timestamps. The result was duplicated sequence numbers, a chain that no longer
-- verified, and a `audit.chain_broken` entry reporting tampering that never happened (E-12 found
-- it with eight concurrent writes; AT-2 writes its thousand entries one after another and could
-- not see it).
--
-- `audit_seq_idx` is what keeps this a lookup rather than a scan of every partition.
SELECT seq, hash
FROM audit_log
WHERE tenant_id = current_tenant_id()
ORDER BY seq DESC
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

-- name: WalkAuditEntries :many
-- One batch of the trail over a period, oldest first, for a verification or an export.
--
-- Ascending, because both callers walk the chain rather than read a screen: the chain is built
-- forwards, and a verifier that met the entries newest first would have to hold the whole period in
-- memory before it could check the first link.
--
-- Ordered by `seq`, which is what the chain is built over - **not** by `occurred_at`, which is what
-- it read until E-12. Every caller takes its own clock reading before it queues for the chain's
-- lock, so two entries can carry timestamps in the opposite order to their sequence numbers. A walk
-- in timestamp order then meets the chain out of order and reports a hash mismatch and a screenful
-- of gaps for a trail that is perfectly intact. The period still selects by `occurred_at` - that is
-- what an operator asks for, and it is what prunes the partitions - and only the order changes.
--
-- Its own statement rather than a direction parameter on the list. A direction that decides an
-- ORDER BY is either two statements behind one name or a string somebody assembles, and the second
-- of those is what rule 9 forbids.
SELECT id, tenant_id, seq, occurred_at, action, outcome, severity,
       actor_type, actor_id, actor_label, on_behalf_of_id,
       target_type, target_id, target_label,
       context, changes, legal_basis, prev_hash, hash
FROM audit_log
WHERE tenant_id = current_tenant_id()
  AND (sqlc.narg('from_time')::timestamptz IS NULL OR occurred_at >= sqlc.narg('from_time')::timestamptz)
  AND (sqlc.narg('to_time')::timestamptz IS NULL OR occurred_at < sqlc.narg('to_time')::timestamptz)
  AND (sqlc.narg('cursor_seq')::bigint IS NULL OR seq > sqlc.narg('cursor_seq')::bigint)
ORDER BY seq ASC
LIMIT sqlc.arg('batch_size');

-- name: LastAuditAnchor :one
-- The last chain end this tenant exported to an append-only target outside the database.
--
-- Nothing writes this table yet, and that is the point of reading it: `:verify` proves the chain is
-- intact *inside* the database, and only an anchor proves anything against somebody who can rewrite
-- the whole of it. `sealed_until` is therefore null on every installation until external anchoring
-- exists (audit.md §3, open point A-2) - null being the honest answer rather than a date that would
-- claim more than the system does.
SELECT anchored_at, last_seq, chain_hash
FROM audit_anchor
WHERE tenant_id = current_tenant_id()
ORDER BY last_seq DESC
LIMIT 1;

-- name: EnsureAuditPartition :one
-- Creates next month's partition if it is not there, and brings any partition that is missing its
-- policy or its revokes back into line (db/migrations/0043_audit_partition_duty.sql).
--
-- The answer is the partition's name, or the empty string when entries for that month are already
-- in the default partition - which PostgreSQL will not split out, and which is an operator's
-- decision rather than a scheduled duty's. Coalesced here rather than answered as NULL, because
-- "no partition was made" is a state the caller acts on rather than an absence it has to unwrap.
SELECT COALESCE(ensure_audit_partition(sqlc.arg('month')::date), '')::text AS partition_name;
