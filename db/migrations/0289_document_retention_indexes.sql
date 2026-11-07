-- What the retention pass reads of pages and boards (data-retention.md §3, accepted.md B): the
-- trashed pages and boards past the trash period, a page's children (a page goes only once none is
-- left under it), and the unnamed page versions past PAGE_VERSION's period.
--
-- Forward-only and safe for a rolling update: indexes only, CONCURRENTLY, outside a transaction,
-- IF NOT EXISTS for an interrupted build (drop the invalid index and run again). Nothing older
-- reads them.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS page_trashed_idx
  ON page (tenant_id, deleted_at) WHERE deleted_at IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS page_children_idx
  ON page (tenant_id, parent_id) WHERE parent_id IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS whiteboard_trashed_idx
  ON whiteboard (tenant_id, deleted_at) WHERE deleted_at IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS page_version_unnamed_idx
  ON page_version (tenant_id, created_at) WHERE name = '';

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
