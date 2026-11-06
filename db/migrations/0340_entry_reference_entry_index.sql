-- The cascade from an entry to the references its text holds (`entry_reference_entry_fkey`) needs
-- an index that leads with its columns. The primary key leads with the source, so each purged
-- entry scanned the tenant's whole entry_reference, and a purge of 500 entries overran the
-- worker's statement timeout on every retry.
--
-- Built CONCURRENTLY on a live table, so no write waits while it builds - which is why this
-- migration takes no transaction.
--
-- Expand only (rule 12): an index nothing has to know about.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS entry_reference_entry_idx
  ON entry_reference (tenant_id, entry_id);

-- +goose Down

-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
