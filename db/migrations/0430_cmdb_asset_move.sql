-- Each step of an asset's life with its moment (UC-AST-12): the state it reached when it was
-- registered and at every move, who moved it, and to whom or which stockroom. cmdb_asset keeps
-- only where it stands now; an asset registered before this table answers that alone, with no
-- date invented for it.
--
-- Expand only (rule 12): a new table nothing older reads. assigned_to is personal data and goes
-- empty with the account, as cmdb_asset's does; moved_by is a bare identifier like updated_by.
-- +goose Up
CREATE TABLE IF NOT EXISTS cmdb_asset_move (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id       uuid NOT NULL,
  ci_id        uuid NOT NULL,
  state        text NOT NULL CHECK (state IN ('REQUESTED', 'ORDERED', 'IN_STOCK', 'DEPLOYED', 'RETIRED', 'DISPOSED')),
  moved_at     timestamptz NOT NULL,
  moved_by     uuid NOT NULL,
  assigned_to  uuid,
  stockroom_id uuid,
  UNIQUE (tenant_id, id),
  CONSTRAINT cmdb_asset_move_asset_fkey FOREIGN KEY (tenant_id, ci_id)
    REFERENCES cmdb_asset (tenant_id, ci_id) ON DELETE CASCADE,
  CONSTRAINT cmdb_asset_move_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT cmdb_asset_move_assigned_fkey FOREIGN KEY (tenant_id, assigned_to)
    REFERENCES account (tenant_id, id) ON DELETE SET NULL (assigned_to),
  CONSTRAINT cmdb_asset_move_stockroom_fkey FOREIGN KEY (tenant_id, stockroom_id)
    REFERENCES cmdb_ci (tenant_id, id) ON DELETE SET NULL (stockroom_id)
);
CREATE INDEX IF NOT EXISTS cmdb_asset_move_ci_idx ON cmdb_asset_move (tenant_id, ci_id, moved_at, id);
CREATE INDEX IF NOT EXISTS cmdb_asset_move_hub_idx ON cmdb_asset_move (tenant_id, hub_id);
CREATE INDEX IF NOT EXISTS cmdb_asset_move_assigned_to_ref_idx
  ON cmdb_asset_move (tenant_id, assigned_to) WHERE assigned_to IS NOT NULL;
CREATE INDEX IF NOT EXISTS cmdb_asset_move_stockroom_idx
  ON cmdb_asset_move (tenant_id, stockroom_id) WHERE stockroom_id IS NOT NULL;

ALTER TABLE cmdb_asset_move ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_asset_move FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_asset_move;
CREATE POLICY tenant_isolation ON cmdb_asset_move
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON cmdb_asset_move TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
