-- The cascades an entry's or a document's purge follows on busy tables need an index that leads
-- with their columns, for 0333's reason: without one each purged entry scans the tenant's
-- notifications, and each purged comment the tenant's comments for its replies.
--
-- Partial on the reference being set: the cascade's `col = $1` implies it, so the planner uses
-- them, and the rows that name nothing - most comments are no reply - cost no index entry.
--
-- Built CONCURRENTLY on live tables, so no write waits while they build - which is why this
-- migration takes no transaction.
--
-- Expand only (rule 12): indexes nothing has to know about.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS notification_item_idx
  ON notification (tenant_id, item_id) WHERE item_id IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS comment_parent_idx
  ON comment (tenant_id, parent_comment_id) WHERE parent_comment_id IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS page_comment_parent_idx
  ON page_comment (tenant_id, parent_id) WHERE parent_id IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS whiteboard_comment_parent_idx
  ON whiteboard_comment (tenant_id, parent_id) WHERE parent_id IS NOT NULL;

-- +goose Down

-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
