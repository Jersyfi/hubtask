-- The backup targets (E-03, backup-restore.md §2).
--
-- backup_target is behind row level security like every other tenant table, so no statement here
-- carries a tenant condition: the policy applies one no query can forget (ADR-0010). What it means
-- for a target with tenant_id IS NULL - the instance-wide shape 0001_init has always allowed - is
-- that nothing in this milestone can see one, because the policy compares against a tenant and an
-- instance-wide row has none. That is deliberate rather than an oversight: instance administration
-- has no surface on this API yet, and the targets a tenant creates are the tenant's.
--
-- The credential is read by exactly one statement, and it is named for it. Every other statement
-- selects the columns a caller may see, so a credential cannot reach a response because somebody
-- added a field to a mapper.

-- name: InsertBackupTarget :exec
INSERT INTO backup_target (
  id, tenant_id, name, kind, config, credential_enc, credential_key_id,
  encryption_mode, encryption_key_id, region_note, insecure_ack_by, insecure_ack_at,
  enabled, created_at, created_by, version
)
VALUES (
  sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('name'), sqlc.arg('kind'),
  sqlc.arg('config'), sqlc.narg('credential_enc'), sqlc.narg('credential_key_id'),
  sqlc.arg('encryption_mode'), sqlc.narg('encryption_key_id'), sqlc.narg('region_note'),
  sqlc.narg('insecure_ack_by'), sqlc.narg('insecure_ack_at'),
  true, sqlc.arg('created_at'), sqlc.arg('created_by'), 1
);

-- The list a client reads back. Ordered by name rather than by creation, because an operator
-- looking for "the Hetzner one" is looking alphabetically.
-- name: ListBackupTargets :many
SELECT id, tenant_id, name, kind, config, credential_key_id, encryption_mode, encryption_key_id,
       region_note, insecure_ack_by, insecure_ack_at, enabled,
       last_test_at, last_test_ok, last_test_error, created_at, created_by, version
FROM backup_target
ORDER BY lower(name);

-- name: FindBackupTarget :one
SELECT id, tenant_id, name, kind, config, credential_key_id, encryption_mode, encryption_key_id,
       region_note, insecure_ack_by, insecure_ack_at, enabled,
       last_test_at, last_test_ok, last_test_error, created_at, created_by, version
FROM backup_target
WHERE id = sqlc.arg('id');

-- The one statement that reads the sealed credential, and it reads nothing else. A separate
-- statement rather than a column on the two above: what a credential must never do is travel with
-- something that is on its way to a response, and the way to make that structural is for the
-- rows that go to a response never to contain one.
-- name: FindBackupTargetCredential :one
SELECT credential_enc, credential_key_id
FROM backup_target
WHERE id = sqlc.arg('id');

-- What the connection probe leaves behind. The error is a message code, never a driver message:
-- an FTP or SSH library's message carries the host, the user and sometimes the password (rule 10).
-- name: RecordBackupTargetTest :execrows
UPDATE backup_target SET
  last_test_at    = sqlc.arg('tested_at'),
  last_test_ok    = sqlc.arg('succeeded'),
  last_test_error = sqlc.narg('error_code')
WHERE id = sqlc.arg('id');

-- Whether this tenant has any target at all, and whether any of them is unencrypted. Two counts
-- rather than a list, because the health surface asks a question about the installation rather
-- than about a target (backup-restore.md §10).
-- name: CountBackupTargets :one
SELECT
  count(*) AS configured,
  count(*) FILTER (WHERE encryption_mode = 'NONE') AS unencrypted
FROM backup_target
WHERE enabled;

-- ─────────────────────────── The schedules and the runs (E-05) ───────────────────────────
--
-- backup_schedule and backup_run are both behind row level security, so no statement here carries
-- a tenant condition - with one exception that is named where it appears: the instance-wide
-- schedules belong to no tenant, and the leader reads them under a scope that has none.

-- name: InsertBackupSchedule :exec
INSERT INTO backup_schedule (
  id, target_id, tenant_id, scope_kind, scope_id, rrule, time_zone, mode, full_rrule,
  include_media, include_audit, retention, notify_on, enabled, next_run_at, created_at, version
)
VALUES (
  sqlc.arg('id'), sqlc.arg('target_id'), sqlc.narg('tenant_id'), sqlc.arg('scope_kind'),
  sqlc.narg('scope_id'), sqlc.arg('rrule'), sqlc.arg('time_zone'), sqlc.arg('mode'),
  sqlc.narg('full_rrule'), sqlc.arg('include_media'), sqlc.arg('include_audit'),
  sqlc.arg('retention'), sqlc.arg('notify_on')::text[], true, sqlc.narg('next_run_at'),
  sqlc.arg('created_at'), 1
);

-- name: ListBackupSchedules :many
SELECT id, target_id, tenant_id, scope_kind, scope_id, rrule, time_zone, mode, full_rrule,
       include_media, include_audit, retention, notify_on, enabled, next_run_at, created_at, version
FROM backup_schedule
ORDER BY created_at;

-- name: FindBackupSchedule :one
SELECT id, target_id, tenant_id, scope_kind, scope_id, rrule, time_zone, mode, full_rrule,
       include_media, include_audit, retention, notify_on, enabled, next_run_at, created_at, version
FROM backup_schedule
WHERE id = sqlc.arg('id');

-- What is due now, in the tenant the transaction is bound to.
--
-- `next_run_at` is a stored decision rather than a rule expanded on the spot: expanding an RRULE
-- costs a library call per schedule, and a poller that did it on every wake-up would do it for
-- every schedule that is not due. The value is written by the pass that last ran, which is the same
-- shape D-03's reminders use.
-- name: DueBackupSchedules :many
SELECT id, target_id, tenant_id, scope_kind, scope_id, rrule, time_zone, mode, full_rrule,
       include_media, include_audit, retention, notify_on, enabled, next_run_at, created_at, version
FROM backup_schedule
WHERE enabled AND next_run_at IS NOT NULL AND next_run_at <= sqlc.arg('now')::timestamptz
ORDER BY next_run_at
LIMIT sqlc.arg('batch')::int;

-- The earliest moment this tenant owes a backup, which is what a poller reschedules itself to.
-- name: NextBackupScheduleDue :one
SELECT min(next_run_at)::timestamptz AS due
FROM backup_schedule
WHERE enabled AND next_run_at IS NOT NULL;

-- name: SetBackupScheduleNextRun :exec
UPDATE backup_schedule
SET next_run_at = sqlc.narg('next_run_at'), version = version + 1
WHERE id = sqlc.arg('id');

-- The runs.
--
-- The insert is the lock §5 asks for: one run at a time per target, enforced by the statement
-- rather than by a check the caller ran a moment earlier - a check followed by an insert has a gap
-- between them wide enough for exactly the thing it prevents.
--
-- `other.id <> id` is what makes a resumption possible. A worker that died left its own row
-- RUNNING; the attempt that takes the job over is the same run, and it has to be able to carry on
-- rather than be locked out by itself (BK-7). ON CONFLICT DO NOTHING is the other half of that: the
-- second attempt writes nothing and reads the row it already has.
-- name: InsertBackupRun :execrows
INSERT INTO backup_run (
  id, schedule_id, target_id, tenant_id, parent_run_id, trigger, mode, status,
  snapshot_at, started_at
)
SELECT
  sqlc.arg('id'), sqlc.narg('schedule_id'), sqlc.arg('target_id'), sqlc.narg('tenant_id'),
  sqlc.narg('parent_run_id'), sqlc.arg('trigger'), sqlc.arg('mode'), 'RUNNING',
  sqlc.narg('snapshot_at'), sqlc.arg('started_at')
WHERE NOT EXISTS (
  SELECT 1 FROM backup_run other
  WHERE other.target_id = sqlc.arg('target_id')
    AND other.status = 'RUNNING'
    AND other.id <> sqlc.arg('id')
)
ON CONFLICT (id) DO NOTHING;

-- name: FindBackupRun :one
SELECT id, schedule_id, target_id, tenant_id, parent_run_id, trigger, mode, status, archive_path,
       size_bytes, item_count, media_count, checksum, snapshot_at, started_at, finished_at,
       error_code, expires_at, verified_at, verify_ok
FROM backup_run
WHERE id = sqlc.arg('id');

-- What the run wrote, once it has written it. The manifest is kept as a copy so that a listing can
-- answer without reaching the target, and the target stays the source of truth (§8.1).
-- name: FinishBackupRun :execrows
UPDATE backup_run SET
  status       = sqlc.arg('status'),
  archive_path = sqlc.narg('archive_path'),
  manifest     = sqlc.narg('manifest'),
  size_bytes   = sqlc.narg('size_bytes'),
  item_count   = sqlc.narg('item_count'),
  media_count  = sqlc.narg('media_count'),
  checksum     = sqlc.narg('checksum'),
  snapshot_at  = COALESCE(sqlc.narg('snapshot_at'), snapshot_at),
  finished_at  = sqlc.arg('finished_at'),
  error_code   = sqlc.narg('error_code')
WHERE id = sqlc.arg('id') AND status = 'RUNNING';

-- The archive an incremental continues: the newest run at this target that finished and left
-- something behind. A run that failed is not a parent - there is no archive at the other end of it.
-- name: LatestSuccessfulBackupRun :one
SELECT id, schedule_id, target_id, tenant_id, parent_run_id, trigger, mode, status, archive_path,
       size_bytes, item_count, media_count, checksum, snapshot_at, started_at, finished_at,
       error_code, expires_at, verified_at, verify_ok
FROM backup_run
WHERE target_id = sqlc.arg('target_id') AND status = 'SUCCEEDED' AND archive_path IS NOT NULL
ORDER BY snapshot_at DESC NULLS LAST, started_at DESC
LIMIT 1;

-- What `:verify` found, without touching anything else on the row.
-- name: RecordBackupVerification :execrows
UPDATE backup_run
SET verified_at = sqlc.arg('verified_at'), verify_ok = sqlc.arg('verify_ok')
WHERE id = sqlc.arg('id');

-- When the generation plan expects an archive to go. Written after a successful run has decided,
-- and cleared for an archive the plan now intends to keep - a stale date on a kept archive is what
-- an operator would read as "this is about to disappear".
-- name: SetBackupRunExpiry :exec
UPDATE backup_run SET expires_at = sqlc.narg('expires_at') WHERE id = sqlc.arg('id');

-- name: ExpireBackupRun :exec
UPDATE backup_run SET status = 'EXPIRED' WHERE id = sqlc.arg('id') AND status = 'SUCCEEDED';

-- The number alert A-12 watches: when each target last had a backup that worked
-- (observability-reliability.md §10). One row per target, and a target that has never had one is
-- absent rather than zero - a gauge of zero reads as "1970" on every dashboard.
-- name: LastSuccessfulBackupPerTarget :many
SELECT target_id, max(COALESCE(finished_at, started_at))::timestamptz AS last_success_at
FROM backup_run
WHERE status = 'SUCCEEDED'
GROUP BY target_id;
