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

-- The devices (N-03, offline-sync.md §6, §10).

-- name: TouchDevice :one
-- Registration and every contact after it in one statement. The insert is the registration; the
-- update is the contact, and it is refused - no row comes back - when the identifier belongs to
-- another account or the device was forgotten, which the adapter then tells apart. What the
-- device said about itself replaces what stood; what it left out leaves it alone.
INSERT INTO sync_device (
  id, tenant_id, account_id, platform, display_name, last_cursor, last_seen_at, credential_id,
  created_at
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'),
  sqlc.narg('platform'), sqlc.narg('display_name'), sqlc.narg('last_cursor'),
  sqlc.arg('now'), sqlc.narg('credential_id'), sqlc.arg('now')
)
ON CONFLICT (id) DO UPDATE SET
  platform      = coalesce(excluded.platform, sync_device.platform),
  display_name  = coalesce(excluded.display_name, sync_device.display_name),
  last_cursor   = greatest(excluded.last_cursor, sync_device.last_cursor),
  last_seen_at  = excluded.last_seen_at,
  credential_id = coalesce(excluded.credential_id, sync_device.credential_id)
WHERE sync_device.tenant_id = current_tenant_id()
  AND sync_device.account_id = excluded.account_id
  AND NOT sync_device.blocked
RETURNING id, tenant_id, account_id, platform, display_name, last_cursor, last_seen_at, blocked,
          created_at, credential_id;

-- name: FindDevice :one
SELECT id, tenant_id, account_id, platform, display_name, last_cursor, last_seen_at, blocked,
       created_at, credential_id
FROM sync_device
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: ListDevicesOfAccount :many
SELECT id, tenant_id, account_id, platform, display_name, last_cursor, last_seen_at, blocked,
       created_at, credential_id
FROM sync_device
WHERE tenant_id = current_tenant_id() AND account_id = sqlc.arg('account_id')
ORDER BY last_seen_at DESC NULLS LAST, created_at DESC;

-- name: ForgetDevice :one
-- The mark, and the row as it was: what the caller revokes is the credential the row held.
UPDATE sync_device
SET blocked = true, last_seen_at = sqlc.arg('now')
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')
  AND account_id = sqlc.arg('account_id') AND NOT blocked
RETURNING id, tenant_id, account_id, platform, display_name, last_cursor, last_seen_at, blocked,
          created_at, credential_id;

-- name: RevokeSessionsOfStaleDevices :execrows
-- §6's other half: a device silent past the period loses its sign-in. The session the device
-- last synchronised under is revoked before the row goes, in the same pass; a credential that
-- is not a session matches nothing here and revokes nothing.
UPDATE session
SET revoked_at = sqlc.arg('now')
WHERE tenant_id = current_tenant_id() AND revoked_at IS NULL
  AND id IN (
    SELECT credential_id FROM sync_device AS stale
    WHERE stale.tenant_id = current_tenant_id()
      AND coalesce(stale.last_seen_at, stale.created_at) < sqlc.arg('cutoff')
      AND stale.credential_id IS NOT NULL
    ORDER BY coalesce(stale.last_seen_at, stale.created_at)
    LIMIT sqlc.arg('batch')
  );

-- name: DeleteStaleDevices :execrows
-- The DEVICE data kind's sweep (data-retention.md §3: anchor `last_seen_at`). Batched through a
-- subquery, oldest first, DeleteExpiredSessions' shape.
DELETE FROM sync_device
WHERE id IN (
  SELECT id FROM sync_device AS stale
  WHERE stale.tenant_id = current_tenant_id()
    AND coalesce(stale.last_seen_at, stale.created_at) < sqlc.arg('cutoff')
  ORDER BY coalesce(stale.last_seen_at, stale.created_at)
  LIMIT sqlc.arg('batch')
);

-- name: CountStaleDevices :one
SELECT count(*) FROM (
  SELECT 1 FROM sync_device AS stale
  WHERE stale.tenant_id = current_tenant_id()
    AND coalesce(stale.last_seen_at, stale.created_at) < sqlc.arg('cutoff')
  LIMIT sqlc.arg('ceiling')
) AS due;
