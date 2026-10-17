-- The installation's subscriptions for a purchase platform and their deliveries (migration 0182).
-- No tenant: read and written by the control plane under the installation's or the system's scope.

-- name: InsertPlatformSubscription :exec
INSERT INTO platform_subscription
  (id, target_url, secret_enc, secret_key_id, state, from_file, created_at, created_by, updated_at)
VALUES
  (sqlc.arg(id), sqlc.arg(target_url), sqlc.arg(secret_enc), sqlc.arg(secret_key_id), 'ACTIVE',
   sqlc.arg(from_file), sqlc.arg(now), sqlc.narg(created_by), sqlc.arg(now));

-- name: ListPlatformSubscriptions :many
SELECT s.id, s.target_url, s.secret_enc, s.secret_key_id, s.state, s.last_error, s.last_delivery_at,
       s.from_file, s.created_at, s.version,
       (SELECT count(*) FROM platform_delivery d
         WHERE d.subscription_id = s.id AND d.status = 'DEAD_LETTER'
           AND d.error_code IS DISTINCT FROM 'webhooks.event_expired')::bigint AS dead_letters
FROM platform_subscription s
ORDER BY s.created_at, s.id;

-- name: FindPlatformSubscription :one
SELECT s.id, s.target_url, s.secret_enc, s.secret_key_id, s.state, s.last_error, s.last_delivery_at,
       s.from_file, s.created_at, s.version,
       (SELECT count(*) FROM platform_delivery d
         WHERE d.subscription_id = s.id AND d.status = 'DEAD_LETTER'
           AND d.error_code IS DISTINCT FROM 'webhooks.event_expired')::bigint AS dead_letters
FROM platform_subscription s
WHERE s.id = sqlc.arg(id);

-- name: ActivePlatformSubscriptionIDs :many
SELECT id FROM platform_subscription WHERE state = 'ACTIVE' ORDER BY id;

-- name: UpdatePlatformSubscription :execrows
-- Guarded on the version read: a change made on stale values is refused, not applied over another.
UPDATE platform_subscription
SET target_url = sqlc.arg(target_url), state = sqlc.arg(state), updated_at = sqlc.arg(now),
    version = version + 1
WHERE id = sqlc.arg(id) AND version = sqlc.arg(expected_version);

-- name: RotatePlatformSubscriptionSecret :execrows
UPDATE platform_subscription
SET secret_enc = sqlc.arg(secret_enc), secret_key_id = sqlc.arg(secret_key_id), updated_at = sqlc.arg(now),
    version = version + 1
WHERE id = sqlc.arg(id);

-- name: MarkPlatformSubscriptionFromFile :exec
UPDATE platform_subscription SET from_file = true WHERE id = sqlc.arg(id);

-- name: DeletePlatformSubscription :execrows
DELETE FROM platform_subscription WHERE id = sqlc.arg(id);

-- name: RecordPlatformSubscriptionOutcome :exec
-- What the last attempt came to, for the screen: the moment of the last arrival, or the last error.
UPDATE platform_subscription
SET last_delivery_at = coalesce(sqlc.narg(delivered_at), last_delivery_at), last_error = sqlc.narg(last_error)
WHERE id = sqlc.arg(id);

-- name: InsertPlatformDelivery :execrows
-- A fan-out repeated for one event writes its delivery once.
INSERT INTO platform_delivery (id, subscription_id, event_id, event_type, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(subscription_id), sqlc.arg(event_id), sqlc.arg(event_type),
        sqlc.arg(now), sqlc.arg(now))
ON CONFLICT (subscription_id, event_id) DO NOTHING;

-- name: FindPlatformDelivery :one
SELECT id, subscription_id, event_id, event_type, attempt, status, response_status, error_code,
       next_attempt_at, created_at, updated_at
FROM platform_delivery WHERE id = sqlc.arg(id);

-- name: PlatformDeliveries :many
SELECT id, subscription_id, event_id, event_type, attempt, status, response_status, error_code,
       next_attempt_at, created_at, updated_at
FROM platform_delivery
WHERE subscription_id = sqlc.arg(subscription_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(before)::uuid IS NULL OR id < sqlc.narg(before)::uuid)
ORDER BY id DESC
LIMIT sqlc.arg(page_size);

-- name: RecordPlatformDeliveryAttempt :execrows
-- One attempt's outcome, written only while the delivery is still the one the attempt read: a
-- repeat of a settled job writes nothing.
UPDATE platform_delivery
SET attempt = sqlc.arg(attempt), status = sqlc.arg(status), response_status = sqlc.narg(response_status),
    error_code = sqlc.narg(error_code), next_attempt_at = sqlc.narg(next_attempt_at), updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND attempt = sqlc.arg(attempt) - 1 AND status IN ('PENDING', 'FAILED');

-- name: RequeueDeadPlatformDeliveries :many
-- A batch of the subscription's dead letters back to pending, each at the start of a fresh round of
-- attempts: the count is raised to the round's boundary, so a round a pause or an expiry cut short
-- does not shorten the next. The status in the statement makes two redeliveries at once queue each
-- dead letter once. A dead letter whose event was swept is not one: nothing can be sent for it.
UPDATE platform_delivery
SET status = 'PENDING', next_attempt_at = NULL, updated_at = sqlc.arg(now),
    attempt = ((attempt + sqlc.arg(round)::int - 1) / sqlc.arg(round)::int) * sqlc.arg(round)::int
WHERE id IN (
  SELECT dead.id FROM platform_delivery AS dead
  WHERE dead.subscription_id = sqlc.arg(subscription_id) AND dead.status = 'DEAD_LETTER'
    AND dead.error_code IS DISTINCT FROM 'webhooks.event_expired'
  ORDER BY dead.id
  LIMIT sqlc.arg(batch)
  FOR UPDATE SKIP LOCKED
)
AND status = 'DEAD_LETTER'
RETURNING id;

-- name: CountPlatformDeadLetters :one
-- The dead letters that can be redelivered: one whose event was swept is not counted, so A-21
-- resolves when the rest are redelivered.
SELECT count(*)::bigint FROM platform_delivery
WHERE status = 'DEAD_LETTER' AND error_code IS DISTINCT FROM 'webhooks.event_expired';

-- name: DeleteExpiredPlatformDeliveries :execrows
DELETE FROM platform_delivery
WHERE id IN (
  SELECT expired.id FROM platform_delivery AS expired
  WHERE expired.updated_at < sqlc.arg(cutoff)::timestamptz AND expired.status <> 'PENDING'
  ORDER BY expired.id
  LIMIT sqlc.arg(batch)
);
