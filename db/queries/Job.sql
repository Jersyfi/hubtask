-- The job queue (ADR-0008). Picked up with FOR UPDATE SKIP LOCKED, so several workers claim
-- disjoint batches instead of queueing behind each other.
--
-- The job table is the one table without row level security (db/migrations/0001_init.sql): part of
-- the queue belongs to no tenant, and a worker must be able to read it before it knows whose job
-- the next one is. Every statement here therefore carries its own conditions - there is no policy
-- underneath that would catch a forgotten one.
--
-- Every statement that finishes an attempt names the lease it is finishing. That is the fence: if
-- the lease expired and somebody else claimed the job in the meantime, the update matches nothing,
-- the caller's transaction rolls back, and the work of the process that fell behind is discarded
-- rather than applied twice (test RT-3).

-- Enqueue, or leave the job that is already scheduled alone. The dedupe key collapses "make sure
-- this is being worked on" into one row; when the waiting job is due later than the new request,
-- its wake-up is pulled forward rather than a second row being created. A job that is already
-- running is not touched: its own reschedule decides when it runs next.
-- name: EnqueueJob :exec
INSERT INTO job (id, tenant_id, kind, payload, dedupe_key, run_at, max_attempts)
VALUES (
  sqlc.arg('id'), sqlc.narg('tenant_id'), sqlc.arg('kind'), sqlc.arg('payload'),
  sqlc.narg('dedupe_key'), sqlc.arg('run_at'), sqlc.arg('max_attempts')
)
ON CONFLICT (kind, dedupe_key) WHERE dedupe_key IS NOT NULL AND state IN ('PENDING','RUNNING')
DO UPDATE SET run_at = LEAST(job.run_at, EXCLUDED.run_at)
WHERE job.state = 'PENDING';

-- The claim. Two kinds of row are claimable: one that is due, and one whose lease has run out -
-- the second is a job whose worker died, and picking it up again is the whole reason a lease has
-- an end.
-- name: ClaimJobs :many
UPDATE job SET
  state        = 'RUNNING',
  attempts     = attempts + 1,
  locked_until = sqlc.arg('locked_until')
WHERE id IN (
  -- The alias is not decoration: without it every column in here reads as a reference to the row
  -- being updated, and "run_at" would mean two different things in one statement.
  SELECT due.id FROM job AS due
  WHERE (due.state = 'PENDING' AND due.run_at <= sqlc.arg('now'))
     OR (due.state = 'RUNNING' AND due.locked_until < sqlc.arg('now'))
  ORDER BY due.priority, due.run_at
  FOR UPDATE SKIP LOCKED
  LIMIT sqlc.arg('batch_size')
)
RETURNING id, tenant_id, kind, payload, attempts, max_attempts, locked_until;

-- name: CompleteJob :execrows
UPDATE job SET
  state        = 'SUCCEEDED',
  finished_at  = sqlc.arg('now'),
  locked_until = NULL,
  last_error   = NULL
WHERE id = sqlc.arg('id') AND state = 'RUNNING' AND locked_until = sqlc.arg('lease');

-- A poller's next round rather than a retry: the attempts start again from zero, because the round
-- that just finished succeeded.
-- name: RepeatJob :execrows
UPDATE job SET
  state        = 'PENDING',
  run_at       = sqlc.arg('run_at'),
  attempts     = 0,
  locked_until = NULL,
  last_error   = NULL
WHERE id = sqlc.arg('id') AND state = 'RUNNING' AND locked_until = sqlc.arg('lease');

-- name: RetryJob :execrows
UPDATE job SET
  state        = 'PENDING',
  run_at       = sqlc.arg('run_at'),
  locked_until = NULL,
  last_error   = sqlc.arg('last_error')
WHERE id = sqlc.arg('id') AND state = 'RUNNING' AND locked_until = sqlc.arg('lease');

-- The dead letter keeps the context an operator needs to act: which kind, which tenant, what it
-- was given, how often it was tried, and the code of the last failure. Never a message - a message
-- can carry what the job was working on (rule 10).
-- name: DeadLetterJob :execrows
UPDATE job SET
  state        = 'DEAD_LETTER',
  finished_at  = sqlc.arg('now'),
  locked_until = NULL,
  last_error   = sqlc.arg('last_error')
WHERE id = sqlc.arg('id') AND state = 'RUNNING' AND locked_until = sqlc.arg('lease');

-- The backlog: what is due and waiting, not what is running. Counting work in flight would make a
-- busy queue look like a stuck one, and the alert hangs off this number rising (alert A-06).
-- name: JobQueueDepth :many
SELECT kind, count(*) AS pending
FROM job
WHERE state = 'PENDING' AND run_at <= sqlc.arg('now')
GROUP BY kind;
