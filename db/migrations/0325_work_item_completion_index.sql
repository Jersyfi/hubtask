-- The index behind a count by completion over a hub or a collection: the live entries of each
-- collection, done and open, read from the index alone. Without it the count read the heap block
-- of every entry it counted - 166 878 blocks for 1.2 million entries (docs/evidence,
-- capacity-10000000-2026-11-01). Narrow, so its repeated keys deduplicate into a few pages per
-- collection. CONCURRENTLY and without a transaction, so no write waits while it builds
-- (versioning-release.md §4); IF NOT EXISTS for a retry after an interrupted build.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS wi_completion_idx
  ON work_item (tenant_id, collection_id, is_completed)
  WHERE deleted_at IS NULL AND archived_at IS NULL;

-- +goose Down

DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
