-- The index behind a board's columns and a publish's counts by status (UC-FLW-02 check 4,
-- UC-FLW-05 check 2). CONCURRENTLY and without a transaction, so no write waits while it builds
-- (versioning-release.md §4); IF NOT EXISTS for a retry after an interrupted build.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS wi_status_idx
  ON work_item (tenant_id, collection_id, status)
  WHERE status IS NOT NULL;

-- +goose Down

DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
