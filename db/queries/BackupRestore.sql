-- The statements a restore reads and records itself through (E-06, backup-restore.md §7, §8).
--
-- Row level security supplies the tenant condition none of these statements writes (ADR-0010),
-- which is BK-10 at the layer where it cannot be forgotten: a restore into another tenant's
-- archive is refused by the use case, and a statement that reached across anyway would find
-- nothing.

-- name: JournalledDeletions :many
-- The deletion journal, read for the first time in production (§7).
--
-- Paged on (deleted_at, entity, entity_id) rather than on OFFSET, for the reason the export is:
-- a page over a set another statement may reorder can repeat a row or drop one, and dropping one
-- here means an object comes back that somebody asked to have deleted.
SELECT entity, entity_id, deleted_at, reason
FROM deletion_journal
WHERE deleted_at > sqlc.arg('since')::timestamptz
  AND (deleted_at, entity, entity_id)
      > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_entity')::text, sqlc.arg('after_id')::uuid)
ORDER BY deleted_at, entity, entity_id
LIMIT sqlc.arg('batch')::int;

-- name: InsertRestoreRun :exec
-- The run row, written in the transaction that accepts the restore (§8.3 step 1). PENDING: the
-- job has not started, and a caller polling `result_url` sees that rather than a 404.
INSERT INTO restore_run (
  id, target_id, source_archive, tenant_id, target_tenant_id, mode, conflict_rule,
  selection, dry_run, create_safety_backup, status, requested_by
) VALUES (
  sqlc.arg('id'), sqlc.arg('target_id'), sqlc.arg('source_archive'), current_tenant_id(),
  sqlc.narg('target_tenant_id'), sqlc.arg('mode'), sqlc.arg('conflict_rule'),
  sqlc.narg('selection'), sqlc.arg('dry_run'), sqlc.arg('create_safety_backup'),
  'PENDING', sqlc.arg('requested_by')
);

-- name: FindRestoreRun :one
SELECT id, target_id, source_archive, tenant_id, target_tenant_id, mode, conflict_rule, selection,
       dry_run, create_safety_backup, safety_backup_run_id, status, report, progress, requested_by,
       approved_by, started_at, finished_at, error_code
FROM restore_run
WHERE id = sqlc.arg('id');

-- name: ClaimRestoreRun :execrows
-- The claim, and the lock against two restores in one tenant at once.
--
-- An UPDATE rather than an insert, because the row is written when the restore is accepted and
-- claimed when the job picks it up - and a job that died and is picked up again has to be able to
-- claim the same row a second time. That is BK-7's restore half: `status IN (...)` includes RUNNING
-- so a resumed attempt continues its own run rather than being told the tenant is busy, and
-- `started_at` is kept rather than moved, so the run still says when it actually began.
UPDATE restore_run run SET
  status     = 'RUNNING',
  started_at = COALESCE(run.started_at, sqlc.arg('started_at')::timestamptz)
WHERE run.id = sqlc.arg('id')
  AND run.status IN ('PENDING', 'VALIDATING', 'RUNNING')
  AND NOT EXISTS (
    SELECT 1 FROM restore_run other
    WHERE other.status IN ('VALIDATING', 'RUNNING')
      AND other.id <> run.id
  );

-- name: FinishRestoreRun :execrows
-- Refused for a run that is no longer going, which is what makes a cancelled restore stay
-- cancelled.
UPDATE restore_run SET
  status               = sqlc.arg('status'),
  -- COALESCE, so that a failure with nothing to report does not erase what the attempt before it
  -- got through. A restore that died in its pre-check knows nothing; the progress the earlier
  -- attempt recorded is the only account of what has been done (BK-7).
  report               = COALESCE(sqlc.narg('report'), report),
  safety_backup_run_id = COALESCE(sqlc.narg('safety_backup_run_id'), safety_backup_run_id),
  finished_at          = sqlc.arg('finished_at'),
  error_code           = sqlc.narg('error_code')
WHERE id = sqlc.arg('id')
  AND status IN ('PENDING', 'VALIDATING', 'RUNNING');

-- name: RecordRestoreSafetyCopy :execrows
-- The copy §8.3 step 4 takes before a destructive mode, recorded before the mode runs: the way back
-- has to be findable from the run even if the run itself then fails.
UPDATE restore_run SET safety_backup_run_id = sqlc.arg('safety_backup_run_id')
WHERE id = sqlc.arg('id');

-- name: RestoreInProgress :one
-- Whether this tenant already has a restore going. Asked when one is requested, so that the refusal
-- is a 409 the caller can read rather than a claim that fails minutes later inside a job.
SELECT EXISTS (
  SELECT 1 FROM restore_run WHERE status IN ('PENDING', 'VALIDATING', 'RUNNING')
) AS running;

-- name: RecordRestoreProgress :execrows
-- How far the run has got, written in the transaction of the batch it got there with (BK-7).
--
-- The report travels with it because a resumed attempt continues counting rather than starting
-- again: a report that only covered the last attempt would say a restore did a fraction of what it
-- did.
UPDATE restore_run SET
  report   = sqlc.arg('report'),
  progress = sqlc.arg('progress')
WHERE id = sqlc.arg('id')
  AND status IN ('PENDING', 'VALIDATING', 'RUNNING');
