-- A template's pages (configuration.md §6): the mark that a page of a hub's space was brought by
-- the hub's document - a guide, a knowledge space's first pages - under its key, so the document
-- reads it back and never brings it twice. The page itself is an ordinary page; the mark leaves
-- with it, and with its hub.
--
-- Expand only (rule 12): a new table nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS template_page (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  page_id    uuid NOT NULL,
  hub_id     uuid NOT NULL,
  key        text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  created_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, page_id),
  UNIQUE (tenant_id, hub_id, key),
  CONSTRAINT template_page_page_fkey FOREIGN KEY (tenant_id, page_id)
    REFERENCES page (tenant_id, id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
  CONSTRAINT template_page_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED
);

ALTER TABLE template_page ENABLE ROW LEVEL SECURITY;
ALTER TABLE template_page FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON template_page;
CREATE POLICY tenant_isolation ON template_page
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON template_page TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
