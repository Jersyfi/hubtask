-- A template's sample entries (configuration.md §6): the mark that an entry was brought by a hub's
-- document to try the way of working on, so one act removes them all. The entry itself is an
-- ordinary entry; the mark leaves with it, and with its hub.
--
-- Expand only (rule 12): a new table nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS sample_entry (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id    uuid NOT NULL,
  hub_id     uuid NOT NULL,
  key        text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  created_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, item_id),
  UNIQUE (tenant_id, hub_id, key),
  CONSTRAINT sample_entry_item_fkey FOREIGN KEY (tenant_id, item_id)
    REFERENCES work_item (tenant_id, id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
  CONSTRAINT sample_entry_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED
);

ALTER TABLE sample_entry ENABLE ROW LEVEL SECURITY;
ALTER TABLE sample_entry FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON sample_entry;
CREATE POLICY tenant_isolation ON sample_entry
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON sample_entry TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
