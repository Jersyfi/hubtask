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
-- Per-tenant round-robin at claim time (H-08, multi-tenancy.md §4): each workspace's due jobs
-- are ranked among themselves, and the batch takes everybody's first before anybody's second -
-- one tenant's storm cannot monopolise the workers, and both keep making progress. Priority and
-- age still order within a rank, so nothing changes whenever only one tenant is due. NULL
-- tenants (system jobs) rank as one workspace of their own.
--
-- The window function cannot share a query level with FOR UPDATE, which is why the lock happens
-- in a second walk over the base table: the CTE ranks a snapshot, the locking read re-checks
-- that each picked row is still due, and SKIP LOCKED keeps two workers on disjoint batches
-- (ADR-0008). The aliases are not decoration: without them "run_at" would mean two different
-- things in one statement.
-- name: ClaimJobs :many
WITH due AS (
  SELECT d.id AS due_id, d.tenant_id AS due_tenant, d.priority AS due_priority,
         d.run_at AS due_run_at
  FROM job AS d
  WHERE (d.state = 'PENDING' AND d.run_at <= sqlc.arg('now'))
     OR (d.state = 'RUNNING' AND d.locked_until < sqlc.arg('now'))
), ranked AS (
  SELECT due_id, due_priority, due_run_at,
         row_number() OVER (PARTITION BY due_tenant ORDER BY due_priority, due_run_at, due_id) AS place
  FROM due
), picked AS (
  SELECT j.id FROM job AS j
  JOIN ranked ON ranked.due_id = j.id
  WHERE (j.state = 'PENDING' AND j.run_at <= sqlc.arg('now'))
     OR (j.state = 'RUNNING' AND j.locked_until < sqlc.arg('now'))
  ORDER BY ranked.place, ranked.due_priority, ranked.due_run_at, ranked.due_id
  LIMIT sqlc.arg('batch_size')
  FOR UPDATE OF j SKIP LOCKED
)
UPDATE job SET
  state        = 'RUNNING',
  attempts     = attempts + 1,
  locked_until = sqlc.arg('locked_until')
WHERE id IN (SELECT id FROM picked)
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

-- The retry ladder parked on the queue: jobs of one kind waiting for a future moment. A webhook
-- retry is exactly one scheduled job - one attempt, one row - so this answers
-- hubtask_webhook_retry_backlog (§4) from the one table that has no tenant boundary to cross.
-- name: ScheduledJobBacklog :one
SELECT count(*) FROM job
WHERE kind = sqlc.arg('kind') AND state = 'PENDING' AND run_at > sqlc.arg('now');

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
