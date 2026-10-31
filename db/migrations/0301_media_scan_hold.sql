-- An installation that scans uploads (security.md §4, T-11) holds a confirmed file PENDING_SCAN
-- while its scanner cannot answer, and marks one a later scan found malicious INFECTED: neither is
-- ever downloaded. The status CHECK widens to admit both.
--
-- Expand only (versioning-release.md §4): an older binary never writes either value, reads one
-- as "not READY" - no download, no attaching - and confirms only a PENDING row, so a rolling
-- update meets nothing it mishandles.

-- +goose Up
ALTER TABLE media_object DROP CONSTRAINT IF EXISTS media_object_status_check;
ALTER TABLE media_object ADD CONSTRAINT media_object_status_check
  CHECK (status IN ('PENDING', 'READY', 'PENDING_SCAN', 'INFECTED'));

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
