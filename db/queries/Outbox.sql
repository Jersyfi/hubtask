-- The transactional outbox (ADR-0007): the event is written in the same transaction as the change
-- it describes, and the dispatcher delivers it afterwards. Nothing here marks an event as
-- delivered - that is the dispatcher's half, and it arrives with A-08.

-- name: AppendOutboxEvent :exec
INSERT INTO outbox_event (
  id, tenant_id, event_type, subject, payload,
  actor_type, actor_id, correlation_id, causation_id, causation_depth, occurred_at
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('event_type'), sqlc.narg('subject'),
  sqlc.arg('payload'), sqlc.arg('actor_type'), sqlc.narg('actor_id'),
  sqlc.narg('correlation_id'), sqlc.narg('causation_id'), sqlc.arg('causation_depth'),
  sqlc.arg('occurred_at')
);
