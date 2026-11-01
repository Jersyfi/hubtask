-- How often a timer's firing failed (domain-model.md §3.7). The firing pass fires each timer under
-- a savepoint of its own: one whose handler fails is put back with a growing delay and counted
-- here, and the pass goes on with the rest, so one bad row no longer holds back every SLA, OLA,
-- approval and page review of its workspace. Expand only (rule 12): a column with a default that
-- a pod of the previous release neither reads nor writes.
-- +goose Up
-- No CHECK: validating one would scan the table under the ALTER's lock; only DeferTimer writes it.
ALTER TABLE durable_timer ADD COLUMN IF NOT EXISTS fire_attempts integer NOT NULL DEFAULT 0;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
