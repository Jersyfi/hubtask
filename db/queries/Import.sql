-- The import from another system (P-08, backup-restore.md §9). Every statement is bounded by the
-- transaction's tenant (SET LOCAL app.tenant_id) and the row level security on import_run.

-- name: InsertImportRun :exec
-- The run row, written in the transaction that accepts the import. PENDING: the job has not
-- started, and a caller polling `result_url` sees that rather than a 404.
INSERT INTO import_run (
  id, tenant_id, requested_by, kind, media_id, hub_id, mapping, time_zone, language, status
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('requested_by'), sqlc.arg('kind'),
  sqlc.arg('media_id'), sqlc.arg('hub_id'), sqlc.narg('mapping'), sqlc.narg('time_zone'),
  sqlc.narg('language'), 'PENDING'
);

-- name: FindImportRun :one
SELECT id, tenant_id, requested_by, kind, media_id, hub_id, mapping, time_zone, language, status,
       report, refused, progress, error_code, created_at, started_at, finished_at
FROM import_run
WHERE id = sqlc.arg('id');

-- name: ClaimImportRun :execrows
-- The claim: a job that died and is picked up again claims the same row a second time, so
-- RUNNING is among the states it may claim from and started_at is kept rather than moved.
UPDATE import_run SET
  status     = 'RUNNING',
  started_at = COALESCE(started_at, sqlc.arg('started_at')::timestamptz)
WHERE id = sqlc.arg('id')
  AND status IN ('PENDING', 'RUNNING');

-- name: RecordImportProgress :execrows
-- How far the run has got, written in the transaction of the batch it got there with, so that a
-- resumed attempt skips what an earlier one decided (the applier's BK-7 rule).
UPDATE import_run SET
  report   = sqlc.arg('report'),
  progress = sqlc.arg('progress')
WHERE id = sqlc.arg('id')
  AND status IN ('PENDING', 'RUNNING');

-- name: FinishImportRun :execrows
-- Refused for a run that is no longer going. COALESCE on the report, so that a failure with
-- nothing to report does not erase what the attempt before it got through.
UPDATE import_run SET
  status      = sqlc.arg('status'),
  report      = COALESCE(sqlc.narg('report'), report),
  refused     = COALESCE(sqlc.narg('refused'), refused),
  error_code  = sqlc.narg('error_code'),
  finished_at = sqlc.arg('finished_at')
WHERE id = sqlc.arg('id')
  AND status IN ('PENDING', 'RUNNING');
