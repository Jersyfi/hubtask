-- Workflows (F-4, UC-FLW-02 … 05): a workspace describes how its work moves - statuses in four
-- categories and the steps between them - and names, per hub or collection and per type, the
-- workflow in force there. An entry in such a collection carries its status, and its completion
-- follows the status's category (D-02).
--
-- A workflow's shape is one document (`published`), and a change in progress a second one
-- (`draft`) that changes nothing until it is published: both are read and written whole and no
-- statement looks inside them. The shipped workflow, keyed `default`, is declared in the domain
-- and gains its row when a workspace first uses or changes it, so no path that creates, restores
-- or imports a workspace has to seed one.
--
-- Expand only (versioning-release.md §4): two new tables and a nullable column. Null is "no
-- workflow runs here", which is what every entry written before this migration, and every entry
-- an older binary writes during the rollout, means. The index follows in 0217, outside a
-- transaction.

-- +goose Up
CREATE TABLE IF NOT EXISTS workflow (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key          text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  system       boolean NOT NULL DEFAULT false,
  -- Empty only on the shipped workflow nobody named: a client shows its own word (rule 8).
  name         text NOT NULL CHECK (char_length(name) <= 120 AND (system OR btrim(name) <> '')),
  published    jsonb NOT NULL
    CHECK (jsonb_typeof(published) = 'object' AND pg_column_size(published) <= 262144),
  draft        jsonb
    CHECK (draft IS NULL OR (jsonb_typeof(draft) = 'object' AND pg_column_size(draft) <= 262144)),
  published_at timestamptz,
  -- Who put the shape in force; kept as an identifier, cleared by an erasure like every author.
  published_by uuid,
  created_at   timestamptz NOT NULL,
  updated_at   timestamptz NOT NULL,
  version      integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT workflow_key_uq UNIQUE (tenant_id, key),
  CHECK (NOT system OR key = 'default')
);

ALTER TABLE workflow ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON workflow
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON workflow TO hubtask_app;

-- The scheme: which workflow a hub or collection runs for one type, or for every type where
-- `entry_type` is empty. A collection's row wins over its hub's, a type's over the row for every
-- type (work.ResolveWorkflow). The reference keeps a workflow in use from being deleted.
CREATE TABLE IF NOT EXISTS workflow_scheme (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  container_id uuid NOT NULL,
  entry_type   text NOT NULL DEFAULT ''
    CHECK (entry_type = '' OR entry_type ~ '^[a-z][a-z0-9_]{0,49}$'),
  workflow_key text NOT NULL,
  created_at   timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, container_id, entry_type),
  CONSTRAINT workflow_scheme_container_fkey
    FOREIGN KEY (tenant_id, container_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT workflow_scheme_workflow_fkey
    FOREIGN KEY (tenant_id, workflow_key) REFERENCES workflow (tenant_id, key)
);

ALTER TABLE workflow_scheme ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_scheme FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON workflow_scheme
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON workflow_scheme TO hubtask_app;

-- The entry's status by key in the workflow in force for it; null where none runs. No CHECK: one
-- on a new column of this table would scan it under its lock, and the domain holds the key's form.
ALTER TABLE work_item ADD COLUMN IF NOT EXISTS status text;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
