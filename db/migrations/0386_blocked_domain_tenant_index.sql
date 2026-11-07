-- A workspace's deletion sets `blocked_domain.tenant_id` to NULL (0211), and its owner reads the
-- block still in view by it (`BlockInView`): both follow the column, which 0211 left without an
-- index. Built CONCURRENTLY without a transaction, as 0371; a leftover invalid index is dropped
-- first.

-- +goose NO TRANSACTION

-- +goose Up

-- +goose StatementBegin
DO $leftover$
BEGIN
  SET LOCAL lock_timeout = '5s';
  IF EXISTS (SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
             WHERE NOT i.indisvalid AND c.relname = 'blocked_domain_tenant_idx') THEN
    DROP INDEX IF EXISTS blocked_domain_tenant_idx;
  END IF;
END
$leftover$;
-- +goose StatementEnd

CREATE INDEX CONCURRENTLY IF NOT EXISTS blocked_domain_tenant_idx
  ON blocked_domain (tenant_id, blocked_at DESC)
  WHERE tenant_id IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
