-- A hub template (configuration/templates.md §1): a template version may carry the part a hub
-- takes - a hub's own document (its module switches, structure, scheme and calendars) beside the
-- workspace's part - and every hub that took it records the version and that part then, the base
-- of the hub's three-way upgrade, as `config_template_use` is the workspace's.
--
-- Expand only (rule 12): a nullable column the previous release neither reads nor writes, and a
-- new table.
-- +goose Up
ALTER TABLE config_template ADD COLUMN IF NOT EXISTS hub_document jsonb;

CREATE TABLE IF NOT EXISTS config_hub_template_use (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id       uuid NOT NULL,
  template_key text NOT NULL,
  version      integer NOT NULL,
  document     jsonb NOT NULL,
  applied_at   timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, hub_id, template_key),
  CONSTRAINT config_hub_template_use_hub_fkey
    FOREIGN KEY (tenant_id, hub_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
ALTER TABLE config_hub_template_use ENABLE ROW LEVEL SECURITY;
ALTER TABLE config_hub_template_use FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON config_hub_template_use;
CREATE POLICY tenant_isolation ON config_hub_template_use
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON config_hub_template_use TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
