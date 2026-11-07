-- Validates 0391's bound on a view's queue position - every row is no queue, or a hub's queue
-- within the bound - and builds the index a hub's queues are read and placed through: partial, so
-- it holds the queues alone. Without a transaction, under locks writes pass.

-- +goose NO TRANSACTION

-- +goose Up
ALTER TABLE saved_view VALIDATE CONSTRAINT saved_view_queue_check;
CREATE INDEX CONCURRENTLY IF NOT EXISTS saved_view_queue_idx
  ON saved_view (tenant_id, scope_id, queue_position) WHERE queue_position IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
