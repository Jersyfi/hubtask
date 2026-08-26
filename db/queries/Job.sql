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
--
-- It answers the identifier of the job that is now scheduled, which is not always the one that was
-- offered: when a dedupe key collapses the request into a job that is already there, the answer is
-- that job's. A 202 has to name something a caller can poll, and naming a row that was never
-- written would be a job resource that answers 404 for work that is happening.
--
-- Zero rows means the conflict met a job that is RUNNING, where the update's WHERE does not fire.
-- The caller then looks the running job up by its key.
-- name: EnqueueJob :many
INSERT INTO job (id, tenant_id, kind, payload, dedupe_key, run_at, max_attempts)
VALUES (
  sqlc.arg('id'), sqlc.narg('tenant_id'), sqlc.arg('kind'), sqlc.arg('payload'),
  sqlc.narg('dedupe_key'), sqlc.arg('run_at'), sqlc.arg('max_attempts')
)
ON CONFLICT (kind, dedupe_key) WHERE dedupe_key IS NOT NULL AND state IN ('PENDING','RUNNING')
DO UPDATE SET run_at = LEAST(job.run_at, EXCLUDED.run_at)
WHERE job.state = 'PENDING'
RETURNING id;

-- The job a dedupe key already names, for the one case the insert above cannot answer for itself.
-- name: FindJobByDedupeKey :one
SELECT id FROM job
WHERE kind = sqlc.arg('kind')::text
  AND dedupe_key = sqlc.arg('dedupe_key')::text
  AND state IN ('PENDING', 'RUNNING')
LIMIT 1;

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

-- The row lock a pass takes on its own job, held until the caller's transaction ends (D-03).
--
-- SELECT ... FOR UPDATE rather than an UPDATE: nothing about the row changes, and what is wanted
-- is only that a concurrent Enqueue on this dedupe key waits for the pass rather than finding the
-- row RUNNING and doing nothing. The lease is in the predicate for the reason every other
-- statement here carries it: a worker that fell behind and lost the job locks nothing.
-- name: HoldJob :one
SELECT id FROM job
WHERE id = sqlc.arg('id') AND state = 'RUNNING' AND locked_until = sqlc.arg('lease')
FOR UPDATE;

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

-- What a caller polls, and the only two statements here that are asked on somebody's behalf
-- rather than by a worker. They therefore carry the tenant condition themselves - the job table
-- has no row level security, so there is no policy underneath that would catch a forgotten one,
-- and a NULL tenant_id (the queue's own housekeeping) matches no caller's tenant by construction.
--
-- The columns are the resource's, not the row's: no payload, no attempt count, no lease, no
-- dedupe key. Selecting them and dropping them later is how one of them eventually reaches a
-- response by accident.
-- name: FindJob :one
SELECT id, tenant_id, state, last_error, created_at, finished_at, progress
FROM job
WHERE id = sqlc.arg('id') AND tenant_id = sqlc.arg('tenant_id');

-- Cancellation, and it is the state change alone: no lease is named, because the caller holds
-- none. A pass that is under way is not interrupted - it finds at its next write that the row is
-- no longer RUNNING under its lease, and rolls back rather than applying its work (test RT-3 is
-- the same fence). The lease is cleared so that nothing waits for it to expire.
--
-- The state condition is what makes a terminal job a conflict rather than a silent overwrite: a
-- job that succeeded a second ago must not read back as cancelled.
-- name: CancelJob :one
UPDATE job SET
  state        = 'CANCELLED',
  finished_at  = sqlc.arg('now'),
  locked_until = NULL
WHERE id = sqlc.arg('id')
  AND tenant_id = sqlc.arg('tenant_id')
  AND state IN ('PENDING', 'RUNNING')
RETURNING id, tenant_id, state, last_error, created_at, finished_at;


-- How far along a long job is, written by the handler as it goes (E-05, migration 0032).
--
-- Fenced on the lease like every other statement a handler runs: a worker that fell so far behind
-- that somebody else took the job over must not keep writing a fraction of work it is no longer
-- doing. A job that is not RUNNING under this lease is simply not updated, and the handler finds
-- out at its next write.
--
-- Clamped in SQL rather than trusted from the caller, because a fraction outside [0,1] on a
-- progress bar is a rendering bug in every client at once.
-- name: SetJobProgress :exec
UPDATE job
SET progress = LEAST(1.0, GREATEST(0.0, sqlc.arg('progress')::real))
WHERE id = sqlc.arg('id')
  AND state = 'RUNNING'
  AND locked_until = sqlc.arg('lease')::timestamptz;
