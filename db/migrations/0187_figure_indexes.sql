-- What a collection's figure reads (UC-WRK-29, domain-model.md §2 "What counts"), indexed. Tenant
-- first, as every row level security predicate compares it first (ADR-0010).
--
-- `wi_figure_open_idx` answers OPEN and FIELD_SUM: the open, unarchived rows directly under a
-- collection. Partial, because a list that has been used for a year holds thousands of ticked
-- rows and a handful of open ones, and the figure is read with every collection in the
-- navigation. `wi_figure_done_idx` answers the week's rhythm: the completions of one collection
-- from a moment on. DUE is served by `wi_due_idx` and PROGRESS by `wi_query_order_idx`, both
-- already there.
--
-- Forward-only and safe for a rolling update: CONCURRENTLY, outside a transaction, IF NOT EXISTS
-- for CONCURRENTLY's interrupted-build failure mode (drop the invalid index and run again).

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS wi_figure_open_idx
  ON work_item (tenant_id, collection_id)
  WHERE parent_id IS NULL AND is_completed = false AND archived_at IS NULL AND deleted_at IS NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS wi_figure_done_idx
  ON work_item (tenant_id, collection_id, completed_at)
  WHERE is_completed = true AND deleted_at IS NULL;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
