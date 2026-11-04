-- A board reads each column as its own ordered page (observability-reliability.md §13.2): the
-- columns' values are found by skipping through these indexes, and each column's first entries
-- are a seek. A board is one collection's entries without the ones below them, so both are partial
-- on `parent_id IS NULL` and serve archived entries too (`include_archived`), which `wi_board_idx`
-- and `wi_status_idx` do not. `order_key COLLATE "C"` for the reason migration 0010 states. Built
-- CONCURRENTLY without a transaction, as 0362; a leftover invalid index is dropped first.

-- +goose NO TRANSACTION

-- +goose Up

-- +goose StatementBegin
DO $leftover$
BEGIN
  SET LOCAL lock_timeout = '5s';
  IF EXISTS (SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
             WHERE NOT i.indisvalid AND c.relname = 'wi_board_bucket_idx') THEN
    DROP INDEX IF EXISTS wi_board_bucket_idx;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
             WHERE NOT i.indisvalid AND c.relname = 'wi_board_status_idx') THEN
    DROP INDEX IF EXISTS wi_board_status_idx;
  END IF;
END
$leftover$;
-- +goose StatementEnd

CREATE INDEX CONCURRENTLY IF NOT EXISTS wi_board_bucket_idx
  ON work_item (tenant_id, collection_id, bucket_id, order_key COLLATE "C", id)
  WHERE deleted_at IS NULL AND parent_id IS NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS wi_board_status_idx
  ON work_item (tenant_id, collection_id, status, order_key COLLATE "C", id)
  WHERE deleted_at IS NULL AND parent_id IS NULL;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
