-- The change log: state deltas for offline clients (offline-sync.md §10). Deliberately separate
-- from the outbox - different recipients, different retention, different compatibility
-- commitments.

-- name: RecordChange :exec
-- `seq` is assigned by the database (GENERATED ALWAYS AS IDENTITY), because the cursor a client
-- pages on has to be gapless and monotonic per tenant. A value chosen in the application would
-- leave holes wherever a transaction rolled back, and a client would then wait for a change that
-- is never coming.
INSERT INTO change_log (
  tenant_id, entity, entity_id, op, container_id, actor_id, device_id, hlc, occurred_at, payload
) VALUES (
  current_tenant_id(), sqlc.arg('entity'), sqlc.arg('entity_id'), sqlc.arg('op'),
  sqlc.narg('container_id'), sqlc.narg('actor_id'), sqlc.narg('device_id'),
  sqlc.arg('hlc'), sqlc.arg('occurred_at'), sqlc.narg('payload')
);

-- name: ReadChangesAfter :many
-- One batch of the change log, in cursor order. What the stream sends and what `:pull` will page.
--
-- `seq > $1` and nothing else: the cursor is a position in a monotonic sequence, so a walk needs no
-- offset, no timestamp comparison and no second sort key. The identity is table-wide rather than
-- per tenant, which makes the sequence sparse for any one of them - a gap between two of a tenant's
-- rows is somebody else's row, never a row of theirs that is missing.
SELECT seq, entity, entity_id, op, container_id, actor_id, device_id, hlc, occurred_at, payload
FROM change_log
WHERE tenant_id = current_tenant_id() AND seq > sqlc.arg('after')
ORDER BY seq
LIMIT sqlc.arg('batch');

-- name: LatestChangeSeq :one
-- Where the log stands now, which is where a client with no cursor starts.
--
-- Zero for a tenant that has never changed anything, and that is the right answer rather than a
-- missing one: a stream opened on an untouched workspace resumes from the beginning of a sequence
-- that has not started, and the first change it is told about is the first change there is.
SELECT coalesce(max(seq), 0)::bigint FROM change_log WHERE tenant_id = current_tenant_id();
