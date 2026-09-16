-- +goose Up
-- The workspace's synchronisation epoch (N-11, backup-restore.md §12 B-5, offline-sync.md §8).
--
-- A restore writes rows without change log entries, so a device that was offline through one
-- keeps a cursor that is still valid and would never be told what changed. The epoch is what
-- tells it: every cursor carries the epoch it was minted under, a restore into an existing
-- workspace - REPLACE_TENANT, MERGE and SELECTIVE alike - advances it as it succeeds, and a
-- cursor from an older epoch is refused as too old, which sends the device through the initial
-- synchronisation and hands it the restored rows. A restore into a new workspace advances
-- nothing: no device holds its cursor yet.
--
-- Expand only. Zero for every workspace, which is the epoch every cursor minted so far belongs to.
ALTER TABLE tenant ADD COLUMN IF NOT EXISTS sync_epoch bigint NOT NULL DEFAULT 0;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
