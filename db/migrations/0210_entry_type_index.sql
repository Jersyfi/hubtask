-- The index behind a filter by type and the in-use question of a type (UC-FLW-01 checks 5, 6), and
-- the validation of the reference 0209 added NOT VALID. CONCURRENTLY and without a transaction, so
-- no write waits while either runs (versioning-release.md §4); IF NOT EXISTS for a retry after an
-- interrupted build.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS wi_entry_type_idx
  ON work_item (tenant_id, entry_type)
  WHERE entry_type IS NOT NULL;

ALTER TABLE work_item VALIDATE CONSTRAINT work_item_entry_type_fkey;

-- +goose Down

DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
