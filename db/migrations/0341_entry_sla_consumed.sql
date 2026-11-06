-- The working time a paused goal had spent, on the requester's and the teams' records. The timer
-- holds it too, but a restore carries no timers (backup-restore.md §8.4) and re-arms each live
-- goal from its record: a running goal from its due moment, a paused one from this. NULL on a
-- record paused before this column existed, which re-arms with its whole target left.
--
-- Expand only (rule 12): a nullable column without a default is a catalogue change, no scan, and
-- a pod of the previous release neither reads nor writes it.

-- +goose Up
ALTER TABLE entry_sla ADD COLUMN IF NOT EXISTS consumed_seconds bigint;
ALTER TABLE entry_ola ADD COLUMN IF NOT EXISTS consumed_seconds bigint;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
