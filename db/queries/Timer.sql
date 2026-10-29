-- Durable timers (migration 0229, domain-model.md §3.7). Every statement runs under the tenant's
-- context (rule 3); row level security keeps them to it.

-- name: InsertTimer :exec
INSERT INTO durable_timer (
  id, tenant_id, purpose, subject_id, name, calendar, budget_seconds, consumed_seconds,
  running_since, due_at, state, fired_at, created_at, updated_at, version
) VALUES (
  sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('purpose'), sqlc.arg('subject_id'),
  sqlc.arg('name'), sqlc.arg('calendar'), sqlc.arg('budget_seconds'), sqlc.arg('consumed_seconds'),
  sqlc.narg('running_since'), sqlc.narg('due_at'), sqlc.arg('state'), sqlc.narg('fired_at'),
  sqlc.arg('now'), sqlc.arg('now'), 1
);

-- The live timer of one purpose, subject and name, locked for the caller's transaction: the write
-- that pauses, resumes or stops it serialises with the firing pass.
-- name: FindLiveTimer :one
SELECT id, tenant_id, purpose, subject_id, name, calendar, budget_seconds, consumed_seconds,
       running_since, due_at, state, fired_at, version
FROM durable_timer
WHERE purpose = sqlc.arg('purpose') AND subject_id = sqlc.arg('subject_id') AND name = sqlc.arg('name')
  AND state IN ('RUNNING', 'PAUSED')
FOR UPDATE;

-- name: UpdateTimer :execrows
UPDATE durable_timer SET
  consumed_seconds = sqlc.arg('consumed_seconds'),
  running_since    = sqlc.narg('running_since'),
  due_at           = sqlc.narg('due_at'),
  state            = sqlc.arg('state'),
  fired_at         = sqlc.narg('fired_at'),
  updated_at       = sqlc.arg('now'),
  version          = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('expected_version');

-- What the firing pass takes: the tenant's running timers whose moment has come, oldest first.
-- FOR UPDATE SKIP LOCKED for the reason the reminders use it: two overlapping passes take
-- disjoint sets, and a write pausing a timer waits for the pass rather than racing it.
-- name: ListDueTimers :many
SELECT id, tenant_id, purpose, subject_id, name, calendar, budget_seconds, consumed_seconds,
       running_since, due_at, state, fired_at, version
FROM durable_timer
WHERE state = 'RUNNING' AND due_at <= sqlc.arg('now')
ORDER BY due_at, id
LIMIT sqlc.arg('batch_size')
FOR UPDATE SKIP LOCKED;

-- The tenant's next due moment, NULL when no timer runs.
-- name: NextTimerDue :one
SELECT min(due_at)::timestamptz AS next_at FROM durable_timer WHERE state = 'RUNNING';
