-- The notification record and what people have said about being told (C-09, arc42 §5.2).
--
-- The tenant is never a parameter: it comes from the transaction's own context through
-- current_tenant_id(), the same value row level security compares against (ADR-0010).

-- name: InsertNotification :execrows
-- The consumer's write, idempotent by the unique index rather than by a read first.
--
-- The outbox delivers at-least-once (ADR-0007), so a consumer may see the same event twice - and
-- asking whether a record exists before writing one is a race two dispatchers both win. The
-- conflict is the answer: zero rows means somebody already wrote it, and the caller does not
-- enqueue a second delivery.
INSERT INTO notification (
  id, tenant_id, recipient_id, category, channel, state, reason,
  event_id, item_id, actor_id, created_at
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('recipient_id'), sqlc.arg('category'),
  sqlc.arg('channel'), sqlc.arg('state'), sqlc.narg('reason'),
  sqlc.narg('event_id'), sqlc.narg('item_id'), sqlc.narg('actor_id'), sqlc.arg('created_at')
)
ON CONFLICT (tenant_id, event_id, recipient_id, channel) WHERE event_id IS NOT NULL
DO NOTHING;

-- name: FindNotification :one
SELECT id, tenant_id, recipient_id, category, channel, state, reason,
       event_id, item_id, actor_id, created_at, sent_at, attempts
FROM notification
WHERE id = $1;

-- name: SaveNotificationOutcome :execrows
-- What the delivery leaves behind: the state, why, when it went, and how often it has been tried.
--
-- The identity, the references and the moment it was written are not in the SET list, and that is
-- the point: a delivery may change what happened to a notification and nothing about what it is
-- about. An UPDATE that could rewrite recipient_id would be a way to send somebody else's message.
UPDATE notification SET
  state    = sqlc.arg('state'),
  reason   = sqlc.narg('reason'),
  sent_at  = sqlc.narg('sent_at'),
  attempts = sqlc.arg('attempts')
WHERE id = sqlc.arg('id')::uuid;

-- name: DeleteExpiredNotifications :execrows
-- The retention sweep's batch (data-retention.md §3: anchor `created_at`, 90 days).
--
-- Batched through a subquery, because DELETE takes no LIMIT: a pass that took every expired row
-- would be a pass nobody can stop, and the sweep's whole shape is one batch per transaction.
-- Oldest first, so a backlog drains in the order it built up.
DELETE FROM notification
WHERE id IN (
  SELECT due.id FROM notification AS due
  WHERE due.created_at < sqlc.arg('cutoff')
  ORDER BY due.created_at
  LIMIT sqlc.arg('batch')
);

-- name: CountExpiredNotifications :one
-- What is due, so a pass can report a backlog it did not get to. Bounded by the batch it would
-- have taken plus one, so a tenant with a million expired rows costs an index scan of a page
-- rather than a count of the table.
SELECT count(*) FROM (
  SELECT 1 FROM notification AS expired
  WHERE expired.created_at < sqlc.arg('cutoff')
  LIMIT sqlc.arg('ceiling')
) AS due;

-- name: FindNotificationPreference :one
SELECT tenant_id, account_id, category, channel, enabled, include_title, updated_at
FROM notification_preference
WHERE account_id = sqlc.arg('account_id')::uuid
  AND category = sqlc.arg('category')
  AND channel = sqlc.arg('channel');

-- name: SaveNotificationPreference :exec
-- A row is an exception rather than a setting, so writing one is an upsert: somebody switching a
-- category off for the second time is stating the same thing, not making a new row.
INSERT INTO notification_preference (
  tenant_id, account_id, category, channel, enabled, include_title, updated_at
) VALUES (
  current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('category'), sqlc.arg('channel'),
  sqlc.arg('enabled'), sqlc.arg('include_title'), sqlc.arg('updated_at')
)
ON CONFLICT (tenant_id, account_id, category, channel) DO UPDATE SET
  enabled       = EXCLUDED.enabled,
  include_title = EXCLUDED.include_title,
  updated_at    = EXCLUDED.updated_at;
