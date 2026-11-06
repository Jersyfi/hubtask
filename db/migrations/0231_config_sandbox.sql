-- The configuration sandbox (config-document.md §3): one copy of a workspace's configuration
-- without content, where an agent may change freely and nothing runs, promoted with a plan.
-- Expand only (versioning-release.md §4): a new table nothing older reads.
--
-- `base_document` is the configuration the sandbox was copied from, so that promoting it plans
-- only what the sandbox changed rather than undoing what changed in force since (a three-way
-- merge, as a template's upgrade). Configuration only, never content and never who wrote it -
-- the trail says who (rule 10).

-- +goose Up
CREATE TABLE IF NOT EXISTS config_sandbox (
  tenant_id     uuid PRIMARY KEY REFERENCES tenant(id) ON DELETE CASCADE,
  document      jsonb NOT NULL,
  base_document jsonb NOT NULL,
  base_hash     text NOT NULL,
  base_version  integer NOT NULL DEFAULT 0,
  writes        integer NOT NULL DEFAULT 0,
  updated_at    timestamptz NOT NULL
);

ALTER TABLE config_sandbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE config_sandbox FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON config_sandbox
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
-- DELETE discards the sandbox.
GRANT SELECT, INSERT, UPDATE, DELETE ON config_sandbox TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
