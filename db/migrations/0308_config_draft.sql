-- Drafts of the configuration's shared blocks (ui.md A4): a type with its form, an SLA policy, a
-- role, saved beside the block in force, which stays in force until the draft is applied. A draft
-- holds the workspace's configuration, never content.
--
-- Expand only (rule 12): a new table nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS config_draft (
  tenant_id uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  section   text NOT NULL CHECK (section IN ('types', 'sla_policies', 'roles')),
  key       text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  block     jsonb NOT NULL
    CHECK (jsonb_typeof(block) = 'object' AND pg_column_size(block) <= 262144),
  saved_by  uuid NOT NULL,
  saved_at  timestamptz NOT NULL,
  version   integer NOT NULL DEFAULT 1,
  PRIMARY KEY (tenant_id, section, key)
);

ALTER TABLE config_draft ENABLE ROW LEVEL SECURITY;
ALTER TABLE config_draft FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON config_draft;
CREATE POLICY tenant_isolation ON config_draft
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON config_draft TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
