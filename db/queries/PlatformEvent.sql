-- The installation's events (UC-INS-14, multi-tenancy.md §4.3; migration 0180).
--
-- Written beside a workspace's change, in that workspace's transaction, and read by the control
-- plane under the installation's scope: the table carries no policy, as the journal does not. The
-- order is the number's, drawn under the lock below, so a reader asking for numbers after the last
-- one it saw never skips a row that commits late.

-- name: LockPlatformEventNumbering :exec
-- Held until the transaction ends. Taken just before the insert rather than at the transaction's
-- start, so that only the tail of a change - from its event to its commit - waits for another. A
-- fixed key beside the scheduler's 8_010_001 rather than a hashed name, which would share its space
-- with the per-workspace hashed keys.
SELECT pg_advisory_xact_lock(8010002);

-- name: InsertPlatformEvent :execrows
-- A writer that may run twice names a dedupe key, and the second run inserts nothing.
INSERT INTO platform_event
  (number, id, event_type, tenant_id, occurred_at, actor_type, actor_id, payload, dedupe_key)
VALUES
  (nextval('platform_event_number'), sqlc.arg(id), sqlc.arg(event_type), sqlc.arg(tenant_id),
   sqlc.arg(occurred_at), sqlc.arg(actor_type), sqlc.narg(actor_id), sqlc.arg(payload),
   sqlc.narg(dedupe_key))
ON CONFLICT (dedupe_key) DO NOTHING;

-- name: PlatformEventsAfter :many
SELECT number, id, event_type, tenant_id, occurred_at, actor_type, actor_id, payload
FROM platform_event
WHERE number > sqlc.arg(after)
ORDER BY number
LIMIT sqlc.arg(page_size);

-- name: ForgetPlatformEventReference :execrows
-- The purge removes the platform's reference from the workspace's kept events, as it removes it
-- from the workspace (multi-tenancy.md §4.3). The workspace is the transaction's, never a
-- parameter.
UPDATE platform_event
SET payload = jsonb_set(payload, '{workspace,external_reference}', 'null'::jsonb)
WHERE tenant_id = current_tenant_id()
  AND payload -> 'workspace' ->> 'external_reference' IS NOT NULL;

-- name: DeleteExpiredPlatformEvents :execrows
DELETE FROM platform_event
WHERE number IN (
  SELECT expired.number FROM platform_event AS expired
  WHERE expired.occurred_at < sqlc.arg(cutoff)::timestamptz
  ORDER BY expired.number
  LIMIT sqlc.arg(batch)
);
