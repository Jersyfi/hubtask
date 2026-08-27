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
