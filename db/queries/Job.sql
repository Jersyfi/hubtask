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
-- Per-tenant round-robin at claim time (multi-tenancy.md §4): each workspace's due jobs
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
--
-- Fair across workers too (F-10): a tenant's ranks start after the jobs it already has running
-- under a live lease, so a workspace that holds every other worker's slot ranks behind one that
-- holds none, and one batch alone no longer decides who is served.
--
-- A role claims only its kinds (deployment.md §2.2): only_kinds empty is every kind, and
-- except_kinds keeps the automation role's kinds off a worker that runs beside one, so a rule
-- storm drains on its own pods. Both arrays are never NULL - `<> ALL(NULL)` would match nothing.
--
-- reclaimed marks a row taken over from a RUNNING attempt whose lease ran out - a worker that died
-- mid-run. The runner counts it and, once such attempts are past max_attempts, dead-letters the job
-- without running it: only a runner that survives records a failure.
-- name: ClaimJobs :many
WITH due AS (
  SELECT d.id AS due_id, d.tenant_id AS due_tenant, d.priority AS due_priority,
         d.run_at AS due_run_at
  FROM job AS d
  WHERE ((d.state = 'PENDING' AND d.run_at <= sqlc.arg('now'))
     OR (d.state = 'RUNNING' AND d.locked_until < sqlc.arg('now')))
    AND (cardinality(sqlc.arg('only_kinds')::text[]) = 0 OR d.kind = ANY(sqlc.arg('only_kinds')::text[]))
    AND d.kind <> ALL(sqlc.arg('except_kinds')::text[])
), busy AS (
  SELECT r.tenant_id AS busy_tenant, count(*) AS running
  FROM job AS r
  WHERE r.state = 'RUNNING' AND r.locked_until >= sqlc.arg('now')
  GROUP BY r.tenant_id
), ranked AS (
  SELECT due_id, due_priority, due_run_at,
         row_number() OVER (PARTITION BY due_tenant ORDER BY due_priority, due_run_at, due_id)
           + coalesce(busy.running, 0) AS place
  FROM due
  LEFT JOIN busy ON busy.busy_tenant IS NOT DISTINCT FROM due.due_tenant
), picked AS (
  SELECT j.id AS picked_id, j.state = 'RUNNING' AS was_running FROM job AS j
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
FROM picked
WHERE job.id = picked.picked_id
RETURNING job.id, job.tenant_id, job.kind, job.payload, job.attempts, job.max_attempts,
          job.locked_until, picked.was_running::boolean AS reclaimed;

-- The row lock a pass takes on its own job, held until the caller's transaction ends.
--
-- SELECT ... FOR UPDATE rather than an UPDATE: nothing about the row changes, and what is wanted
-- is only that a concurrent Enqueue on this dedupe key waits for the pass rather than finding the
-- row RUNNING and doing nothing. The lease is in the predicate for the reason every other
-- statement here carries it: a worker that fell behind and lost the job locks nothing.
-- name: HoldJob :one
SELECT id FROM job
WHERE id = sqlc.arg('id') AND state = 'RUNNING' AND locked_until = sqlc.arg('lease')
FOR UPDATE;

-- A claimed job's lease moved to a later moment, before the job starts: a batch runs one job after
-- another, and a job must not begin with less lease left than it may run. Fenced like every
-- statement here: a job another worker took over after the lease ran out is not extended.
-- name: ExtendJobLease :execrows
UPDATE job SET locked_until = sqlc.arg('locked_until')
WHERE id = sqlc.arg('id') AND state = 'RUNNING' AND locked_until = sqlc.arg('lease');

-- The outcome is the handler's message code, or NULL where its kind has none to say (rule 10: a
-- code, never content).
-- name: CompleteJob :execrows
UPDATE job SET
  state        = 'SUCCEEDED',
  finished_at  = sqlc.arg('now'),
  locked_until = NULL,
  last_error   = NULL,
  outcome      = sqlc.narg('outcome')
WHERE id = sqlc.arg('id') AND state = 'RUNNING' AND locked_until = sqlc.arg('lease');

-- A poller's next round rather than a retry: the attempts start again from zero, because the round
-- that just finished succeeded.
-- name: RepeatJob :execrows
UPDATE job SET
  state        = 'PENDING',
  run_at       = sqlc.arg('run_at'),
  attempts     = 0,
  revivals     = 0,
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

-- A refusal the job met - an AI budget spent since it was queued, AI switched off - ends it as
-- FAILED rather than in the dead letter: it is an answer for whoever asked, not work waiting for an
-- operator. The parameters are the refusal's own, codes and moments (rule 10).
-- name: RefuseJob :execrows
UPDATE job SET
  state             = 'FAILED',
  finished_at       = sqlc.arg('now'),
  locked_until      = NULL,
  last_error        = sqlc.arg('last_error'),
  last_error_params = sqlc.narg('last_error_params')
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

-- The dead letters waiting for an operator, every kind and workspace: the health report's
-- dead_letter_total, which an installation without Prometheus reads instead of alert A-07.
-- name: DeadLetterCount :one
SELECT count(*) FROM job WHERE state = 'DEAD_LETTER';

-- The jobs of one kind not yet finished, due, parked between rounds or running: for a kind with one
-- job per workspace while there is work (media.scan), the number of workspaces that have it.
-- name: OpenJobCount :one
SELECT count(*) FROM job
WHERE kind = sqlc.arg('kind') AND state IN ('PENDING', 'RUNNING');

-- The keys of one duty kind with work left: an open job, or a dead letter no later success of its
-- key settled - which the hourly revival takes up again. For media.scan, one key per workspace,
-- the workspaces whose uploads stay held, a dead-lettered pass included (alert A-23).
-- name: UnsettledDutyCount :one
SELECT count(DISTINCT dedupe_key) FROM job AS duty
WHERE duty.kind = sqlc.arg('kind')
  AND (duty.state IN ('PENDING', 'RUNNING')
    OR (duty.state = 'DEAD_LETTER' AND NOT EXISTS (
      SELECT 1 FROM job AS later
      WHERE later.kind = duty.kind AND later.dedupe_key = duty.dedupe_key
        AND later.state = 'SUCCEEDED' AND later.finished_at > duty.finished_at)));

-- The same keys for media.scan as UnsettledDutyCount, with how long the oldest has waited: the
-- health report's notice of held uploads. A repeated pass keeps its row, so created_at is when the
-- workspace's hold began.
-- name: HeldUploadDuties :one
SELECT count(DISTINCT dedupe_key)::bigint AS workspaces,
       COALESCE(EXTRACT(EPOCH FROM (sqlc.arg('now')::timestamptz - min(created_at))), 0)::float8 AS oldest_seconds
FROM job AS duty
WHERE duty.kind = 'media.scan'
  AND (duty.state IN ('PENDING', 'RUNNING')
    OR (duty.state = 'DEAD_LETTER' AND NOT EXISTS (
      SELECT 1 FROM job AS later
      WHERE later.kind = duty.kind AND later.dedupe_key = duty.dedupe_key
        AND later.state = 'SUCCEEDED' AND later.finished_at > duty.finished_at)));

-- The workspace and key of each open job of one kind, by turns: every workspace's oldest before
-- any workspace's second, so a limit below the total leaves no workspace out while another holds
-- several (mail.mailbox_poll, keyed by the mailbox: the things that live now). No payload: the
-- caller loads each under its own workspace.
-- name: OpenJobKeys :many
SELECT open_key.tenant_id, open_key.dedupe_key FROM (
  SELECT tenant_id, dedupe_key::text AS dedupe_key, created_at, id,
         row_number() OVER (PARTITION BY tenant_id ORDER BY created_at, id) AS turn
  FROM job
  WHERE kind = sqlc.arg('kind') AND state IN ('PENDING', 'RUNNING')
    AND tenant_id IS NOT NULL AND dedupe_key IS NOT NULL
) AS open_key
ORDER BY open_key.turn, open_key.created_at, open_key.id
LIMIT sqlc.arg('limit_count');

-- How many open jobs of one kind OpenJobKeys could answer, beyond any limit.
-- name: CountOpenJobKeys :one
SELECT count(*)::bigint FROM job
WHERE kind = sqlc.arg('kind') AND state IN ('PENDING', 'RUNNING')
  AND tenant_id IS NOT NULL AND dedupe_key IS NOT NULL;

-- What a caller polls, and the only two statements here that are asked on somebody's behalf
-- rather than by a worker. They therefore carry the tenant condition themselves - the job table
-- has no row level security, so there is no policy underneath that would catch a forgotten one,
-- and a NULL tenant_id (the queue's own housekeeping) matches no caller's tenant by construction.
--
-- The columns are the resource's, not the row's: no payload, no attempt count, no lease, no
-- dedupe key. Selecting them and dropping them later is how one of them eventually reaches a
-- response by accident.
-- name: FindJob :one
SELECT id, tenant_id, state, last_error, last_error_params, outcome, created_at, finished_at, progress
FROM job
WHERE id = sqlc.arg('id') AND tenant_id = sqlc.arg('tenant_id');

-- Who asked a job, and about what: the kind and the three references a question to AI carries in
-- its payload. The tenant is named in the statement, as in every statement here, because the
-- table has no policy underneath. Empty strings where a payload names none.
-- name: FindJobAsker :one
SELECT kind,
       coalesce(payload->>'asked_by', '')::text AS asked_by,
       coalesce(payload->>'target_type', '')::text AS target_type,
       coalesce(payload->>'target_id', '')::text AS target_id
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


-- How far along a long job is, written by the handler as it goes (migration 0032).
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

-- name: CallJobsSealedNotUnder :many
-- The HTTP calls a re-seal visits (ADR-0045): a job that may still run whose header secret names a
-- key other than the current one. Only the current workspace's - the table has no policy to say so.
SELECT id, payload
FROM job
WHERE tenant_id = current_tenant_id()
  AND kind = 'automation.http'
  AND state IN ('PENDING', 'RUNNING')
  AND payload #>> '{secret_header_sealed,key_id}' <> sqlc.arg('key_id')::text
ORDER BY id;

-- name: RewrapCallJobSecret :execrows
-- Only the sealed secret, guarded by the key it named when it was read: a job that finished, or
-- that another round moved, in between is left alone. The lease columns are not touched, so a
-- worker holding the job finishes it as it would have.
UPDATE job
SET payload = jsonb_set(payload, '{secret_header_sealed}', sqlc.arg('sealed')::jsonb)
WHERE id = sqlc.arg('id')
  AND tenant_id = current_tenant_id()
  AND kind = 'automation.http'
  AND state IN ('PENDING', 'RUNNING')
  AND payload #>> '{secret_header_sealed,key_id}' = sqlc.arg('expected_key_id')::text;

-- A duty the dead letter holds goes back to work (queue.Perpetual): the same row, due now, its
-- attempts from zero, once it has waited an hour doubled by each earlier revival and capped at a
-- day - unless its key already has an open job, which then is the duty. Bounded, oldest first; the
-- answer is the kinds revived, never a payload (rule 10).
-- name: ReviveDeadDuties :many
UPDATE job SET
  state        = 'PENDING',
  run_at       = sqlc.arg('now')::timestamptz,
  attempts     = 0,
  revivals     = job.revivals + 1,
  finished_at  = NULL,
  locked_until = NULL
WHERE job.id IN (
  SELECT dead.id FROM job AS dead
  WHERE dead.state = 'DEAD_LETTER'
    AND dead.kind = ANY(sqlc.arg('kinds')::text[])
    AND dead.finished_at <= sqlc.arg('now')::timestamptz
      - LEAST(interval '1 hour' * power(2, LEAST(dead.revivals, 5)), interval '24 hours')
    AND (dead.dedupe_key IS NULL OR NOT EXISTS (
      SELECT 1 FROM job AS open
      WHERE open.kind = dead.kind AND open.dedupe_key = dead.dedupe_key
        AND open.state IN ('PENDING', 'RUNNING')))
    -- Only a key's newest dead letter, so one statement never revives two rows of one key.
    AND (dead.dedupe_key IS NULL OR NOT EXISTS (
      SELECT 1 FROM job AS newer
      WHERE newer.kind = dead.kind AND newer.dedupe_key = dead.dedupe_key
        AND newer.state = 'DEAD_LETTER' AND (newer.finished_at, newer.id) > (dead.finished_at, dead.id)))
  ORDER BY dead.finished_at, dead.id
  LIMIT sqlc.arg('batch')::int
  FOR UPDATE SKIP LOCKED
)
RETURNING kind;

-- The catalogue's period for a finished job: seven days after it ended (docs/privacy/data-catalog.md).
-- Every terminal state alike, dead letters included - a payload names people by identifier, and a
-- dead letter has had A-07 for the week. Oldest first and bounded, so a backlog clears over runs
-- rather than in one statement; no tenant, because the table has none (multi-tenancy.md §2.1).
-- name: PurgeFinishedJobs :many
DELETE FROM job
WHERE id IN (
  SELECT id FROM job
  WHERE state IN ('SUCCEEDED', 'FAILED', 'DEAD_LETTER', 'CANCELLED')
    AND finished_at < sqlc.arg('before')::timestamptz
  ORDER BY finished_at, id
  LIMIT sqlc.arg('batch')::int
)
RETURNING state;

-- The oldest open transaction's age in seconds (migration 0331, alert A-25).
-- name: OldestTransactionSeconds :one
SELECT oldest_transaction_seconds()::float8 AS seconds;

-- The indexes PostgreSQL holds invalid (alert A-28): an index a CREATE INDEX CONCURRENTLY left
-- behind when it was interrupted, which every write still maintains and no query uses, and which
-- a unique index's constraint does not enforce. The migrator rebuilds the ones a migration builds
-- (versioning-release.md §4); one that stays is what this counts. pg_index is readable by every
-- role, so no function is needed.
-- name: InvalidIndexCount :one
SELECT count(*)::bigint AS invalid
FROM pg_catalog.pg_index i
JOIN pg_catalog.pg_class c ON c.oid = i.indexrelid
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public' AND NOT i.indisvalid;

-- What would let the connected role past row level security (multi-tenancy.md §2.1): a superuser
-- or BYPASSRLS role, a role that owns tables with RLS - the doors' standing owner policies then
-- apply to it - and a backfill's policy left behind (`%_backfill`, TO the migrating role). The
-- catalogues are readable by every role.
-- name: DatabaseRoleIsolation :one
SELECT r.rolsuper AS superuser, r.rolbypassrls AS bypass_rls,
  EXISTS (
    SELECT 1 FROM pg_catalog.pg_class c
    JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public' AND c.relrowsecurity AND c.relowner = r.oid
  ) AS owns_tables,
  (SELECT count(*) FROM pg_catalog.pg_policies p WHERE p.policyname LIKE '%\_backfill')::bigint AS backfill_policies
FROM pg_catalog.pg_roles r
WHERE r.rolname = current_user;

-- The installation's dead letters for an operator (UC-INS-12), newest first: codes and moments,
-- never the payload (rule 10). The table has no policy; only the operator's door reads this.
-- name: PageDeadLetters :many
SELECT id, tenant_id, kind, attempts, max_attempts, last_error, created_at, finished_at
FROM job
WHERE state = 'DEAD_LETTER'
  AND (sqlc.narg('kind')::text IS NULL OR kind = sqlc.narg('kind')::text)
  AND (sqlc.narg('before')::uuid IS NULL OR id < sqlc.narg('before')::uuid)
ORDER BY id DESC
LIMIT sqlc.arg('size')::int;

-- One dead letter back to the queue as it was, due now, its attempts from zero - unless its key
-- already has an open job, which then does the work (job_dedupe_uq would refuse the row anyway).
-- name: RequeueDeadLetter :one
UPDATE job SET
  state        = 'PENDING',
  run_at       = sqlc.arg('now')::timestamptz,
  attempts     = 0,
  finished_at  = NULL,
  locked_until = NULL,
  last_error   = NULL,
  last_error_params = NULL
WHERE job.id = sqlc.arg('id') AND job.state = 'DEAD_LETTER'
  AND (job.dedupe_key IS NULL OR NOT EXISTS (
    SELECT 1 FROM job AS open
    WHERE open.kind = job.kind AND open.dedupe_key = job.dedupe_key
      AND open.state IN ('PENDING', 'RUNNING')))
RETURNING id, tenant_id, kind, attempts, max_attempts, last_error, created_at;

-- Whether a job of this identifier is a dead letter, for the refusal a requeue that changed nothing
-- answers.
-- name: IsDeadLetter :one
SELECT EXISTS (SELECT 1 FROM job WHERE id = sqlc.arg('id') AND state = 'DEAD_LETTER');

-- How close the database is to the limits that stop every write (alerts A-36, A-37): the share of
-- the NOTIFY queue a stuck listener has filled - once full, every transaction that notifies fails
-- at commit - and the transaction-ID age vacuum has not yet frozen, which at about two billion
-- makes PostgreSQL refuse writes (migration 0353); and the database's size on disk, which alert
-- A-42 holds against the operator's budget before the volume or the plan runs out.
-- name: DatabaseHorizon :one
SELECT pg_notification_queue_usage()::float8 AS notify_queue_usage, frozen_xid_age()::bigint AS xid_age,
  pg_database_size(current_database())::bigint AS database_bytes;

-- How the last invitation mail to each of these accounts went (0362): the newest job of the
-- invitation kind per account, whose dedupe key is the account. The tenant is named, as in every
-- statement here. No payload is read: it holds identifiers only, and the answer needs none.
--
-- The job only records the notification and queues its delivery, so once it has succeeded what
-- the mail did is the newest INVITATION notification of the account written since (0363): sent,
-- failed or suppressed, with its reason code. Account identifiers are compared as text, the form
-- the dedupe key has.
-- name: LatestInvitationDeliveries :many
SELECT j.account_id, j.state, j.last_error, j.last_error_params, j.created_at, j.finished_at,
       coalesce(n.state, '')::text AS mail_state, n.reason AS mail_reason, n.sent_at AS mail_sent_at
FROM (
  SELECT DISTINCT ON (jj.dedupe_key)
         jj.tenant_id AS job_tenant_id, jj.dedupe_key::text AS account_id, jj.state, jj.last_error,
         jj.last_error_params, jj.created_at, jj.finished_at
  FROM job jj
  WHERE jj.tenant_id = sqlc.arg('tenant_id')::uuid
    AND jj.kind = 'notification.invitation'
    AND jj.dedupe_key = ANY(sqlc.arg('account_ids')::text[])
  ORDER BY jj.dedupe_key, jj.created_at DESC, jj.id DESC
) j
LEFT JOIN LATERAL (
  SELECT nn.state, nn.reason, nn.sent_at
  FROM notification nn
  WHERE nn.tenant_id = j.job_tenant_id
    AND nn.category = 'INVITATION'
    AND nn.recipient_id::text = j.account_id
    AND nn.created_at >= j.created_at
  ORDER BY nn.created_at DESC
  LIMIT 1
) n ON true;

-- The invitation mail to one account being sent right now (identity.md §12, "Never both"). Its
-- delivery is detached and mints its token outside the runner's transaction, so cancelling it
-- would not stop the token: a hand-over by link waits until it has finished. The tenant is named,
-- as in every statement here.
-- name: InvitationMailSending :one
SELECT EXISTS (
  SELECT 1 FROM job j
  JOIN notification n ON n.tenant_id = j.tenant_id AND n.id::text = j.dedupe_key
  WHERE j.tenant_id = sqlc.arg('tenant_id')::uuid
    AND j.kind = 'notification.deliver'
    AND j.state = 'RUNNING'
    AND n.category = 'INVITATION'
    AND n.recipient_id = sqlc.arg('account_id')::uuid
)::boolean AS sending;

-- Cancels the invitation mail to one account still waiting before its invitation is handed over
-- by link: the invitation job not yet run (which records nothing once cancelled; one under way
-- rolls back at its next write) and the delivery of a recorded invitation not yet sent. The
-- delivery mints a token of its own and the newest wins, so a mail left waiting would kill the
-- handed-over link.
-- name: CancelOpenInvitationMail :execrows
UPDATE job j SET
  state        = 'CANCELLED',
  finished_at  = sqlc.arg('now'),
  locked_until = NULL
WHERE j.tenant_id = sqlc.arg('tenant_id')::uuid
  AND j.state IN ('PENDING', 'RUNNING')
  AND (
    (j.kind = 'notification.invitation' AND j.dedupe_key = sqlc.arg('account_id')::uuid::text)
    OR (j.kind = 'notification.deliver' AND j.state = 'PENDING' AND j.dedupe_key IN (
      SELECT n.id::text FROM notification n
      WHERE n.tenant_id = sqlc.arg('tenant_id')::uuid AND n.category = 'INVITATION'
        AND n.recipient_id = sqlc.arg('account_id')::uuid AND n.state = 'PENDING'))
  );

-- The recorded invitation mails to one account not yet sent, set aside for the link that replaced
-- them, so nothing sends them later and the people list says why they did not go.
-- name: SupersedePendingInvitationMail :execrows
UPDATE notification SET state = 'SUPPRESSED', reason = 'notifications.replaced_by_link'
WHERE tenant_id = sqlc.arg('tenant_id')::uuid AND category = 'INVITATION'
  AND recipient_id = sqlc.arg('account_id')::uuid AND state = 'PENDING';
