-- The collection an import lands in, where it lands in one rather than as new collections under
-- the hub (backup-restore.md §9). `hub_id` stays the hub: the collection's own, so that reading the
-- run is still decided on the hub it landed in.
--
-- Expand only: a nullable column without a default, so the statement rewrites nothing, and a
-- process of the previous release neither writes nor reads it. Such a run is enqueued under a job
-- kind of its own, so that a pod of the previous release, which would land the file as new
-- collections, never claims it (rule 12). No foreign key, as `hub_id` has none: the run is
-- bookkeeping and outlives a collection that is deleted afterwards.

-- +goose Up
ALTER TABLE import_run ADD COLUMN IF NOT EXISTS collection_id uuid;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
