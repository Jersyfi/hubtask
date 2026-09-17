-- The trial restore in the default schedule (B-4, P-14, backup-restore.md §5).
--
-- A backup schedule gains `trial_restore`: when it is on, the run that wrote a FULL archive is
-- followed, in the same job, by an INSPECT restore of that archive - every member read, every
-- checksum verified, every encrypted member decrypted with the key the schedule names, the
-- difference report produced against the workspace - and a trial that fails fails the run,
-- because an archive the product cannot read is not a backup.
--
-- On for a new schedule and off for every schedule that exists: the column is added with a default
-- of false so the rows already there keep the behaviour they had, and the default is then set to
-- true for the rows written from now on. An existing schedule is switched on by its owner through
-- the ordinary update, which is where a change in what a nightly job does belongs. The run keeps
-- the trial's report beside its manifest. Expand only: two nullable-or-defaulted columns, read
-- and written by the new version alone.

-- +goose Up

ALTER TABLE backup_schedule ADD COLUMN IF NOT EXISTS trial_restore boolean NOT NULL DEFAULT false;
ALTER TABLE backup_schedule ALTER COLUMN trial_restore SET DEFAULT true;

ALTER TABLE backup_run ADD COLUMN IF NOT EXISTS trial_report jsonb;
ALTER TABLE backup_run ADD COLUMN IF NOT EXISTS trial_at timestamptz;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
