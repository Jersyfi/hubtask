-- The installation's own backup targets (backup-restore.md §2, multi-tenancy.md §5).
--
-- A `backup_target` row with no workspace has always been allowed (ADR-0019: targets are the
-- operator's business) and has never been reachable: the one policy compares against a tenant, and
-- such a row has none. This adds the door migration 0129 gave `ai_provider`: a session with no
-- tenant set - the operator's `SystemScope` for writing, `InstallationScope` for reading - reaches
-- the rows with no tenant, and only those. A session with a tenant set still reaches its own rows
-- alone, so no workspace lists, names or opens an installation target.
--
-- No read policy admitting the installation's rows to a workspace, unlike `ai_provider`: a
-- workspace has no use for one. The export, the one job that writes to one, opens it in the
-- installation's scope itself.
--
-- Expand-only: a policy added, nothing changed for a row that has a tenant.

-- +goose Up
CREATE POLICY installation_targets ON backup_target FOR ALL
  USING (tenant_id IS NULL AND current_tenant_id() IS NULL)
  WITH CHECK (tenant_id IS NULL AND current_tenant_id() IS NULL);

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
