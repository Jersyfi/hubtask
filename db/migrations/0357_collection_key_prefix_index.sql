-- The second half of 0356 on a table that holds data: the key prefix's unique index built
-- CONCURRENTLY and its CHECK validated, without a transaction, so each statement takes only locks
-- writes pass. The trigger of 0356 keeps the prefixes apart before the index exists; the index is
-- the backstop under two concurrent writers. A leftover invalid index of an interrupted build is
-- dropped first, so a retry builds it again.

-- +goose NO TRANSACTION

-- +goose Up

-- +goose StatementBegin
DO $leftover$
BEGIN
  SET LOCAL lock_timeout = '5s';
  IF EXISTS (SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
             WHERE NOT i.indisvalid AND c.relname = 'container_key_prefix_uq') THEN
    DROP INDEX IF EXISTS container_key_prefix_uq;
  END IF;
END
$leftover$;
-- +goose StatementEnd

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS container_key_prefix_uq
  ON container (tenant_id, key_prefix) WHERE key_prefix IS NOT NULL;
ALTER TABLE container VALIDATE CONSTRAINT container_key_prefix_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
