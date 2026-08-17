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
