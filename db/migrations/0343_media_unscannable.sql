-- A held upload the scanner refuses for its size is settled UNSCANNABLE - never served - instead
-- of staying held at the front of every scan pass (security.md §4, T-11). The status CHECK widens
-- to admit it.
--
-- Expand only (versioning-release.md §4): an older binary never writes the value and reads it as
-- "not READY" - no download. Without a transaction, so the ADD's short exclusive lock (bounded by
-- the migrator's lock_timeout) is released before the VALIDATE reads the table under a lock writes
-- pass.

-- +goose NO TRANSACTION

-- +goose Up

ALTER TABLE media_object DROP CONSTRAINT IF EXISTS media_object_status_check,
  ADD CONSTRAINT media_object_status_check
  CHECK (status IN ('PENDING', 'READY', 'PENDING_SCAN', 'INFECTED', 'UNSCANNABLE')) NOT VALID;

ALTER TABLE media_object VALIDATE CONSTRAINT media_object_status_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
