-- A workflow defined at a hub (configuration model §1, §2; UC-FLW-02 check 9): only the hub and
-- its collections may run a hub's own workflow, and a copy of a shared one records the key it was
-- made from. Keys stay unique in the workspace, as 0324 chose for entry types, so a scheme row
-- still names one workflow (`workflow_scheme_workflow_fkey`) and a copy takes a key of its own.
--
-- The hub goes with its workflows: a hub is purged with its collections and their scheme rows in
-- one cascade, and a collection whose own row runs its hub's workflow never leaves the hub (the
-- move is refused), so nothing outside still names one. `copied_from` is a key, not a reference:
-- the copy keeps saying where it came from after the source is gone.
--
-- Expand only (rule 12): two nullable columns, the reference added NOT VALID and validated after,
-- so the table is not held while it is read. Null is the whole workspace, which is what every
-- workflow written before this migration, and every one an older binary writes, means.

-- +goose Up
ALTER TABLE workflow ADD COLUMN IF NOT EXISTS hub_id uuid;
ALTER TABLE workflow ADD COLUMN IF NOT EXISTS copied_from text
  CHECK (copied_from IS NULL OR copied_from ~ '^[a-z][a-z0-9_]{0,49}$');
ALTER TABLE workflow
  ADD CONSTRAINT workflow_hub_fkey
  FOREIGN KEY (tenant_id, hub_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE NOT VALID;
ALTER TABLE workflow VALIDATE CONSTRAINT workflow_hub_fkey;
-- The shipped workflow is the whole workspace's.
ALTER TABLE workflow
  ADD CONSTRAINT workflow_system_workspace_check CHECK (NOT system OR hub_id IS NULL) NOT VALID;
ALTER TABLE workflow VALIDATE CONSTRAINT workflow_system_workspace_check;
CREATE INDEX IF NOT EXISTS workflow_hub_idx ON workflow (tenant_id, hub_id) WHERE hub_id IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
