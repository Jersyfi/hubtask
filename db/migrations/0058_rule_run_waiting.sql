-- A run parked on a WAIT (G-09, automation.md §1.3).
--
-- WAIT suspends the run rather than sleeping on a worker: its results so far are written, a job
-- carries the resume point with the queue's own run_at, and the current job finishes. The parked
-- run needs its own status, because a row left in RUNNING is how a crash is recognised - a run
-- that is deliberately waiting a day must not read as one.
--
-- Forward-only and safe for a rolling update: the CHECK constraint is replaced by name, the new
-- set is a superset of the old, and NOT VALID plus VALIDATE keeps both locks brief. A release
-- still running the old binary never writes 'WAITING', and every row it writes passes the new
-- constraint.

-- +goose Up

ALTER TABLE rule_run
  DROP CONSTRAINT IF EXISTS rule_run_status_check;
ALTER TABLE rule_run
  ADD CONSTRAINT rule_run_status_check
  CHECK (status IN ('RUNNING','WAITING','SUCCEEDED','SKIPPED','FAILED','ABORTED_LOOP','THROTTLED'))
  NOT VALID;
ALTER TABLE rule_run VALIDATE CONSTRAINT rule_run_status_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
