-- +goose Up
-- Close the runs whose jobs the queue had already given up on (#207).
--
-- Until the Release hook existed, a backup.run or backup.restore job that went to the dead letter
-- left its run row open - RUNNING for a backup, PENDING/VALIDATING/RUNNING for a restore - and the
-- open row is a lock: one run per target, one restore per tenant. Nothing in production ever read
-- those rows again, so an installation that had hit the defect stayed locked until somebody edited
-- the database by hand. From this release the dead letter closes its own row; this closes the ones
-- it left behind.
--
-- The guard is the job table, not an age: a row whose job is still PENDING or RUNNING belongs to
-- work that is alive - a backup mid-flight during this very migration - and is left alone. The job
-- table survives its jobs (the dead letter is a state, not a deletion), so a row without a living
-- job is one nobody is coming back for. Data only; no schema object changes.
UPDATE backup_run
SET status = 'FAILED', finished_at = now(), error_code = 'backup.run_abandoned'
WHERE status = 'RUNNING'
  AND NOT EXISTS (
    SELECT 1 FROM job
    WHERE job.kind = 'backup.run'
      AND job.state IN ('PENDING', 'RUNNING')
      AND job.payload->>'run_id' = backup_run.id::text
  );

UPDATE restore_run
SET status = 'FAILED', finished_at = now(), error_code = 'backup.restore_abandoned'
WHERE status IN ('PENDING', 'VALIDATING', 'RUNNING')
  AND NOT EXISTS (
    SELECT 1 FROM job
    WHERE job.kind = 'backup.restore'
      AND job.state IN ('PENDING', 'RUNNING')
      AND job.payload->>'restore_id' = restore_run.id::text
  );
