-- The reads C-07 adds, indexed (ADR-0010: tenant first, because that is what every row level
-- security predicate compares first).
--
-- `cfd_scope_idx` is the list: the definitions in force for one collection are its own and the
-- workspace-wide ones above it, which is one walk over (tenant_id, collection_id). Partial on the
-- live rows, because a deleted definition is never in an answer - its values stay in the entries
-- and stop being visible, which is the whole shape of the soft delete.
--
-- `custom_field_definition_tenant_id_uq` is the tenant-first unique index every table a
-- tenant-scoped foreign key could point at carries (ADR-0024, migration 0004). Nothing references
-- the definitions yet - a value is a key in a jsonb document rather than a row with a key - and
-- the index is here so that the first thing that wants to does not have to add it under load.
--
-- Forward-only and safe for a rolling update: CONCURRENTLY, outside a transaction, IF NOT EXISTS
-- for CONCURRENTLY's interrupted-build failure mode (drop the invalid index and run again).

-- +goose NO TRANSACTION

-- +goose Up

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS custom_field_definition_tenant_id_uq
  ON custom_field_definition (tenant_id, id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS cfd_scope_idx
  ON custom_field_definition (tenant_id, collection_id) WHERE deleted_at IS NULL;
