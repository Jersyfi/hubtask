-- A goal period's deletion cascades to `config_goal_set` (0440), and a reply's to the replies
-- under it (`post_reply.parent_id`, 0401): both follow columns no index led with, so each deleted
-- parent scanned the tenant's rows (known-traps.md, "A cascade without an index"). Built
-- CONCURRENTLY without a transaction, as 0386; a leftover invalid index is dropped first.

-- +goose NO TRANSACTION

-- +goose Up

-- +goose StatementBegin
DO $leftover$
BEGIN
  SET LOCAL lock_timeout = '5s';
  IF EXISTS (SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
             WHERE NOT i.indisvalid AND c.relname = 'config_goal_set_period_idx') THEN
    DROP INDEX IF EXISTS config_goal_set_period_idx;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
             WHERE NOT i.indisvalid AND c.relname = 'post_reply_parent_idx') THEN
    DROP INDEX IF EXISTS post_reply_parent_idx;
  END IF;
END
$leftover$;
-- +goose StatementEnd

CREATE INDEX CONCURRENTLY IF NOT EXISTS config_goal_set_period_idx
  ON config_goal_set (tenant_id, period_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS post_reply_parent_idx
  ON post_reply (tenant_id, parent_id)
  WHERE parent_id IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
