-- Webhook subscriptions and their deliveries (G-03, automation.md §3.1).
--
-- The tenant is never a parameter: row level security bounds every statement to the tenant of the
-- running transaction, which is what makes another workspace's subscription invisible rather than
-- forbidden (ADR-0010, multi-tenancy.md §2).
--
-- The sealed secrets travel as a ciphertext and a key identifier together. A sealed value is a
-- pair - the envelope opens under whichever master key sealed it - and an installation that has
-- rotated its keyring holds several (E-02).

-- name: InsertWebhookSubscription :exec
INSERT INTO webhook_subscription (
  id, tenant_id, target_url, event_types, filter_expr,
  secret_enc, secret_key_id, state, created_by, created_at, version
)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('target_url'), sqlc.arg('event_types'),
  sqlc.narg('filter_expr'), sqlc.arg('secret_enc'), sqlc.narg('secret_key_id'),
  sqlc.arg('state'), sqlc.arg('created_by'), sqlc.arg('created_at'), 1
);

-- name: FindWebhookSubscription :one
SELECT id, target_url, event_types, filter_expr, secret_enc, secret_key_id,
       previous_secret_enc, previous_secret_key_id, previous_secret_until,
       state, failure_count, last_error, disabled_at, created_by, created_at, version
FROM webhook_subscription
WHERE id = sqlc.arg('id');

-- name: ListWebhookSubscriptions :many
-- Newest first by identifier: UUIDv7 is time-ordered, so the primary key is the creation order.
-- Not paged - a workspace has a handful of integrations, and a cursor over a handful is machinery
-- nobody reads.
SELECT id, target_url, event_types, filter_expr, secret_enc, secret_key_id,
       previous_secret_enc, previous_secret_key_id, previous_secret_until,
       state, failure_count, last_error, disabled_at, created_by, created_at, version
FROM webhook_subscription
ORDER BY id DESC;

-- name: SubscriptionsForEventType :many
-- What the dispatcher asks per event: the active subscriptions that named this type.
--
-- Filtered in the database rather than in the process, because the alternative is reading every
-- subscription of the tenant on every event. The array containment is what the whole shape of
-- `event_types` is for.
SELECT id, target_url, event_types, filter_expr, secret_enc, secret_key_id,
       previous_secret_enc, previous_secret_key_id, previous_secret_until,
       state, failure_count, last_error, disabled_at, created_by, created_at, version
FROM webhook_subscription
WHERE state = 'ACTIVE' AND event_types @> ARRAY[sqlc.arg('event_type')::text]
ORDER BY id;

-- name: UpdateWebhookSubscription :execrows
-- Optimistic locking in the WHERE clause: the update matches nothing when somebody else has moved
-- the row on, and the caller learns that rather than overwriting them (api-guidelines.md).
UPDATE webhook_subscription SET
  target_url  = sqlc.arg('target_url'),
  event_types = sqlc.arg('event_types'),
  filter_expr = sqlc.narg('filter_expr'),
  state       = sqlc.arg('state'),
  failure_count = sqlc.arg('failure_count'),
  last_error  = sqlc.narg('last_error'),
  disabled_at = sqlc.narg('disabled_at'),
  version     = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('expected_version');

-- name: RotateWebhookSecret :execrows
-- The rotation, as one statement: the new secret becomes current, the current one becomes previous
-- with the moment it stops verifying, and the version moves. Two statements could leave a
-- subscription with a new secret and no grace, which is the failure the grace exists to prevent.
UPDATE webhook_subscription SET
  previous_secret_enc    = secret_enc,
  previous_secret_key_id = secret_key_id,
  previous_secret_until  = sqlc.narg('previous_secret_until'),
  secret_enc             = sqlc.arg('secret_enc'),
  secret_key_id          = sqlc.narg('secret_key_id'),
  version                = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('expected_version');

-- name: DeleteWebhookSubscription :execrows
-- The deliveries go with it, by cascade (0001_init): a delivery log for a subscription nobody can
-- reach any more is a record of attempts against an address the workspace no longer knows.
DELETE FROM webhook_subscription WHERE id = sqlc.arg('id');

-- ============================== Deliveries ==============================

-- name: InsertWebhookDelivery :exec
INSERT INTO webhook_delivery (
  id, tenant_id, subscription_id, event_id, attempt, status, next_attempt_at, created_at
)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('subscription_id'), sqlc.arg('event_id'),
  sqlc.arg('attempt'), sqlc.arg('status'), sqlc.narg('next_attempt_at'), sqlc.arg('created_at')
);

-- name: FindWebhookDelivery :one
SELECT id, subscription_id, event_id, attempt, status, response_status, error_code,
       next_attempt_at, created_at
FROM webhook_delivery
WHERE id = sqlc.arg('id');

-- name: WebhookDeliveries :many
-- One subscription's attempts, newest first, optionally narrowed to one outcome - DEAD_LETTER is
-- the one an operator usually wants. The cursor is the identifier, which is time-ordered.
SELECT id, subscription_id, event_id, attempt, status, response_status, error_code,
       next_attempt_at, created_at
FROM webhook_delivery
WHERE subscription_id = sqlc.arg('subscription_id')
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('before')::uuid IS NULL OR id < sqlc.narg('before')::uuid)
ORDER BY id DESC
LIMIT sqlc.arg('page_size');

-- name: RecordWebhookDeliveryOutcome :execrows
-- What became of one attempt. The response status is the target's own; the error is a message code
-- of ours, never the target's response body (rule 10).
UPDATE webhook_delivery SET
  status          = sqlc.arg('status'),
  response_status = sqlc.narg('response_status'),
  error_code      = sqlc.narg('error_code'),
  next_attempt_at = sqlc.narg('next_attempt_at')
WHERE id = sqlc.arg('id');
