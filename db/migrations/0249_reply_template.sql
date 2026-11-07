-- Canned replies (M5, UC-SVC-06): a workspace's reply templates under a key, the text a handler
-- sends often with placeholders the entry fills. A new table, so expand only
-- (versioning-release.md §4).
-- +goose Up
CREATE TABLE IF NOT EXISTS reply_template (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key         text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  body        text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 20000),
  created_at  timestamptz NOT NULL,
  updated_at  timestamptz NOT NULL,
  version     integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT reply_template_key_uq UNIQUE (tenant_id, key)
);
ALTER TABLE reply_template ENABLE ROW LEVEL SECURITY;
ALTER TABLE reply_template FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON reply_template;
CREATE POLICY tenant_isolation ON reply_template
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON reply_template TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
