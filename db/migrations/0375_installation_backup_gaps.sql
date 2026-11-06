-- How many workspaces have no backup (UC-BAK-01, observability-reliability.md §5).
--
-- Hubtask creates no backup target or schedule by itself, so a fresh installation makes no backup
-- until somebody chooses where they go. The health report warns while a workspace has no enabled
-- target or no successful backup in the last seven days; nothing may read targets or runs across
-- workspaces, so `installation_backup_gaps()` is a door of `instance_census`'s shape (0105):
-- numbers only, never a workspace, a target or a path. The installation's own targets and runs
-- (no tenant) cover every workspace, since they back up the whole installation. Only active
-- workspaces count.
--
-- The owner reads `backup_target` and `backup_run` across workspaces by standing policies named
-- `TO CURRENT_USER` (0159's reasoning); `hubtask_app` gains nothing but the function.
--
-- Expand only and safe for a rolling update: policies and a function are added; a pod of the
-- previous release never calls it.

-- +goose Up
-- +goose StatementBegin
DO $owner$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'backup_target' AND policyname = 'doors_read') THEN
    CREATE POLICY doors_read ON backup_target FOR SELECT TO CURRENT_USER USING (true);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'backup_run' AND policyname = 'doors_read') THEN
    CREATE POLICY doors_read ON backup_run FOR SELECT TO CURRENT_USER USING (true);
  END IF;
END $owner$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION installation_backup_gaps(stale_days integer)
RETURNS TABLE (without_target bigint, without_recent bigint)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT
    count(*) FILTER (WHERE NOT EXISTS (
      SELECT 1 FROM backup_target AS target
      WHERE target.enabled AND (target.tenant_id = workspace.id OR target.tenant_id IS NULL))),
    count(*) FILTER (WHERE NOT EXISTS (
      SELECT 1 FROM backup_run AS run
      WHERE run.status = 'SUCCEEDED' AND run.finished_at > now() - make_interval(days => stale_days)
        AND (run.tenant_id = workspace.id OR run.tenant_id IS NULL)))
  FROM tenant AS workspace
  WHERE workspace.status = 'ACTIVE'
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION installation_backup_gaps(integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION installation_backup_gaps(integer) TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
