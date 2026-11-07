-- How often the scheduler's leader has brought a duty back from the dead letter since its last
-- round that worked (queue.Perpetual): the wait before the next revival doubles with it, from one
-- hour to a day, so a duty that keeps failing for good costs a few attempts a day, never a loop.
-- A round that works sets it back to zero.
--
-- Expand only (rule 12): a column with a constant default is a catalogue change, no scan, and a
-- pod of the previous release neither reads nor writes it.

-- +goose Up
ALTER TABLE job ADD COLUMN IF NOT EXISTS revivals integer NOT NULL DEFAULT 0;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
