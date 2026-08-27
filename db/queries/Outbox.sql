-- The transactional outbox (ADR-0007): the event is written in the same transaction as the change
-- it describes, and the dispatcher delivers it afterwards.
--
-- Both halves are here, and both run under row level security: an event is written into the tenant
-- of the transaction, and read out of it again. A dispatcher therefore works one tenant at a time
-- (multi-tenancy.md §2.1) - which is also what stops one busy tenant from filling every batch.

-- name: AppendOutboxEvent :exec
INSERT INTO outbox_event (
  id, tenant_id, event_type, subject, payload,
  actor_type, actor_id, correlation_id, causation_id, causation_depth, occurred_at, replay
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('event_type'), sqlc.narg('subject'),
  sqlc.arg('payload'), sqlc.arg('actor_type'), sqlc.narg('actor_id'),
  sqlc.narg('correlation_id'), sqlc.narg('causation_id'), sqlc.arg('causation_depth'),
  sqlc.arg('occurred_at'), sqlc.arg('replay')
);

-- The dispatcher's claim. The rows are locked for the length of the transaction and rows another
-- dispatcher holds are skipped rather than waited for, so two workers on one tenant divide the
-- work instead of queueing.
-- name: ClaimPendingEvents :many
SELECT
  id, tenant_id, event_type, subject, payload,
  actor_type, actor_id, correlation_id, causation_id, causation_depth, occurred_at, replay
FROM outbox_event
WHERE dispatched_at IS NULL
ORDER BY occurred_at, id
FOR UPDATE SKIP LOCKED
LIMIT sqlc.arg('batch_size');

-- Recorded in the same transaction as the delivery itself. A mark that committed without its
-- delivery would lose the event silently, which is the one failure an outbox exists to rule out.
-- name: MarkEventsDispatched :exec
UPDATE outbox_event
SET dispatched_at = sqlc.arg('dispatched_at')
WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- What is left for this tenant after a round: the dispatcher's decision between running again at
-- once and going back to sleep.
-- name: CountPendingEvents :one
SELECT count(*) FROM outbox_event WHERE dispatched_at IS NULL;

-- The claim is the question (ADR-0007): a consumer inserts before it reacts, and an insert that
-- changes nothing is the answer "somebody already has". Asking first and inserting afterwards
-- would let two dispatchers both be told no.
-- name: ClaimEventConsumption :execrows
INSERT INTO event_consumption (tenant_id, consumer, event_id, consumed_at)
VALUES (current_tenant_id(), sqlc.arg('consumer'), sqlc.arg('event_id'), sqlc.arg('consumed_at'))
ON CONFLICT DO NOTHING;

-- name: DeleteDispatchedEvents :execrows
-- The retention sweep's batch (data-retention.md §3: anchor `occurred_at`, 7 days).
--
-- The guard is `dispatched_at IS NOT NULL`, and it is a correctness rule rather than a retention
-- one: the dispatcher stamps that column only after every subscriber has had the event, so a NULL
-- means somebody has not consumed it yet. Such a row is never due, whatever period a tenant
-- configures - deleting it would lose the event silently, which is the one failure an outbox
-- exists to rule out (ADR-0007).
--
-- Batched through a subquery, because DELETE takes no LIMIT: a pass that took every expired row
-- would be a pass nobody can stop. Oldest first, so a backlog drains in the order it built up.
DELETE FROM outbox_event
WHERE id IN (
  SELECT due.id FROM outbox_event AS due
  WHERE due.dispatched_at IS NOT NULL
    AND due.occurred_at < sqlc.arg('cutoff')
  ORDER BY due.occurred_at
  LIMIT sqlc.arg('batch')
);

-- name: CountDispatchedEvents :one
-- How many rows are due, counted no higher than the ceiling: what the caller needs is "is there
-- more after this batch" rather than a count of the table.
SELECT count(*) FROM (
  SELECT 1 FROM outbox_event
  WHERE dispatched_at IS NOT NULL
    AND occurred_at < sqlc.arg('cutoff')
  LIMIT sqlc.arg('ceiling')
) AS due;

-- name: DeleteExpiredConsumption :execrows
-- The other half of the outbox's sweep: the record of who has already consumed what
-- (core/port/eventbus.RetentionWindow).
--
-- event_consumption_gc_idx has existed since phase 0 and nothing ever collected against it. The
-- table is the outbox's twin - one row per event per consumer - so leaving it unswept would have
-- made the sweep of the events themselves a half measure.
--
-- The same period as the events, deliberately: a record whose event has been swept can say nothing
-- about an event nobody can deliver again. Two periods that could drift apart would give one of
-- them a value at which this stops being true.
DELETE FROM event_consumption
WHERE (tenant_id, consumer, event_id) IN (
  SELECT due.tenant_id, due.consumer, due.event_id FROM event_consumption AS due
  WHERE due.consumed_at < sqlc.arg('cutoff')
  ORDER BY due.consumed_at
  LIMIT sqlc.arg('batch')
);

-- name: FindOutboxEvent :one
-- One event, as it was written. The webhook deliverer renders the body from this rather than from
-- a copy in the job payload, so a retry two days later sends what the first attempt would have -
-- and a job row does not become a second place a workspace's content lives.
SELECT id, tenant_id, event_type, subject, payload, actor_type, actor_id,
       correlation_id, causation_id, causation_depth, occurred_at, replay
FROM outbox_event
WHERE id = sqlc.arg('id');

-- name: PollOutboxEvents :many
-- The pull half of the stream (G-04, automation.md §3.2): one type, oldest first, from a position.
--
-- Three predicates and each is a boundary the endpoint has to draw.
--
-- `replay = false` is the same rule the push half keeps: a restore's events go to nobody
-- outward-facing, or a restore would report last month's states to every trigger (migration 0033).
--
-- `occurred_at <= horizon` is what makes the cursor gapless. The order is `(occurred_at, id)`, and
-- `occurred_at` comes from the writing transaction rather than from its commit - so a transaction
-- that began before one already answered can still commit a row that sorts *behind* the cursor,
-- and a poller would step over it and never know. The horizon is a moment far enough back that no
-- such transaction can still be open; rows newer than it are withheld from the page and from the
-- cursor together, and are answered by the next poll. Withholding an event for a few seconds is a
-- delay, and stepping over it is a loss.
--
-- The keyset itself is the row comparison rather than `occurred_at > $1 OR (= AND id > $2)`: one
-- comparison the index can seek on, where the disjunction is two it cannot.
--
-- The two sides of the keyset are cast, because a row comparison gives sqlc nothing to infer a
-- parameter's type from: without them `after_id` is generated as a timestamp, and the mistake is one
-- the compiler would not catch until it was a uuid being written into a timestamptz.
--
-- No `dispatched_at` predicate. A poll answers what has been delivered as readily as what has not:
-- the pull half is a second transport, not a second delivery.
SELECT
  id, tenant_id, event_type, subject, payload,
  actor_type, actor_id, correlation_id, causation_id, causation_depth, occurred_at, replay
FROM outbox_event
WHERE event_type = sqlc.arg('event_type')
  AND replay = false
  AND occurred_at <= sqlc.arg('horizon')
  AND (occurred_at, id) > (sqlc.arg('after_occurred_at')::timestamptz, sqlc.arg('after_id')::uuid)
ORDER BY occurred_at, id
LIMIT sqlc.arg('batch');
