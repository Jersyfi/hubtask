-- Range and sort over an IP_ADDRESS field (0298): the `addr` column per field, as `efv_num_idx`
-- serves a number.
--
-- Forward-only and safe for a rolling update: an index only, CONCURRENTLY, outside a transaction,
-- IF NOT EXISTS for an interrupted build (drop the invalid index and run again). Nothing older
-- reads it.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS efv_addr_idx
  ON entry_field_value (tenant_id, definition_id, addr) WHERE addr IS NOT NULL;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
