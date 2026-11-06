-- An entry type defined at a hub (configuration model §1, §2; UC-FLW-01 check 10): a hub's own
-- type is offered only inside that hub, and a copy of a shared type records the key it was made
-- from. Keys stay unique in the workspace, so an entry's type still names one row
-- (`work_item_entry_type_fkey`) and a copy takes a key of its own.
--
-- The hub goes with its types: a hub is purged with its collections and entries in one cascade,
-- and an entry carrying a hub's type never leaves the hub (the move is refused), so nothing
-- outside still names one. `copied_from` is a key, not a reference: the copy keeps saying where
-- it came from after the source is gone.
--
-- Expand only (rule 12): two nullable columns, the reference added NOT VALID and validated after,
-- so the table is not held while it is read. Null is the whole workspace, which is what every
-- type written before this migration, and every one an older binary writes, means.

-- +goose Up
ALTER TABLE entry_type ADD COLUMN IF NOT EXISTS hub_id uuid;
ALTER TABLE entry_type ADD COLUMN IF NOT EXISTS copied_from text
  CHECK (copied_from IS NULL OR copied_from ~ '^[a-z][a-z0-9_]{0,49}$');
ALTER TABLE entry_type
  ADD CONSTRAINT entry_type_hub_fkey
  FOREIGN KEY (tenant_id, hub_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE NOT VALID;
ALTER TABLE entry_type VALIDATE CONSTRAINT entry_type_hub_fkey;
-- A system type is the whole workspace's.
ALTER TABLE entry_type
  ADD CONSTRAINT entry_type_system_workspace_check CHECK (NOT system OR hub_id IS NULL) NOT VALID;
ALTER TABLE entry_type VALIDATE CONSTRAINT entry_type_system_workspace_check;
CREATE INDEX IF NOT EXISTS entry_type_hub_idx ON entry_type (tenant_id, hub_id) WHERE hub_id IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
