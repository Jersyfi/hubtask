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
