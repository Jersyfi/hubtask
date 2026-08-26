-- +goose Up
-- data-retention.md §6: "An automatic export before deletion is possible (EXPORT_THEN_DELETE) - the
-- archive lands at the configured backup target." That archive is a backup run like any other, and
-- `backup_run.trigger` has four values, none of which is it: SCHEDULE is the timer, MANUAL is a
-- person, PRE_RESTORE is the safety copy of backup-restore.md §8.3, and API is a caller.
--
-- Writing one of those instead would put the retention engine's exports in with somebody else's,
-- and "who wrote this archive, and why" is the question a trigger exists to answer.
--
-- Widening a check constraint is safe for a rolling update in both directions: an old process never
-- writes the new value, and a new one never reads a row the old one would refuse.
ALTER TABLE backup_run DROP CONSTRAINT backup_run_trigger_check;
ALTER TABLE backup_run ADD CONSTRAINT backup_run_trigger_check
  CHECK (trigger IN ('SCHEDULE', 'MANUAL', 'PRE_RESTORE', 'PRE_DELETE', 'API'));

-- +goose Down
-- Development only; in production the rule is to fix forwards (versioning-release.md §4).
ALTER TABLE backup_run DROP CONSTRAINT backup_run_trigger_check;
ALTER TABLE backup_run ADD CONSTRAINT backup_run_trigger_check
  CHECK (trigger IN ('SCHEDULE', 'MANUAL', 'PRE_RESTORE', 'API'));
