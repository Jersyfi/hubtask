-- Three repairs to the suite's own tables that the integration tests found.
--
-- * `dashboard_digest.dashboard_id` referenced `dashboard` on its identifier alone, so a digest in
--   one workspace could name, and a cascade could delete across, a dashboard in another (ADR-0024).
--   It now references (tenant_id, id), which needs the tenant-first unique index on `dashboard`
--   every referenced table carries.
-- * A baseline's entry count is written after its entries are copied (`SetPlanBaselineCount`); the
--   application may update that one column and nothing else of a baseline.
-- * A replace-restore empties every table of the archive before it writes the copy back, and the
--   agents' tables were granted no DELETE: the restore stopped at the first of them.
--
-- Expand only (rule 12): an index, a key that admits exactly the rows the old one did - a digest's
-- tenant is its dashboard's by the row level policy of both - and grants; a process of the previous
-- release does nothing the new ones refuse. The key is added NOT VALID and validated, so no row is
-- locked while the table is scanned.

-- +goose Up
CREATE UNIQUE INDEX IF NOT EXISTS dashboard_tenant_id_uq ON dashboard (tenant_id, id);

ALTER TABLE dashboard_digest DROP CONSTRAINT IF EXISTS dashboard_digest_dashboard_fkey;
ALTER TABLE dashboard_digest ADD CONSTRAINT dashboard_digest_dashboard_fkey
  FOREIGN KEY (tenant_id, dashboard_id) REFERENCES dashboard (tenant_id, id) ON DELETE CASCADE NOT VALID;
ALTER TABLE dashboard_digest VALIDATE CONSTRAINT dashboard_digest_dashboard_fkey;
ALTER TABLE dashboard_digest DROP CONSTRAINT IF EXISTS dashboard_digest_dashboard_id_fkey;

GRANT UPDATE (entry_count) ON plan_baseline TO hubtask_app;

GRANT DELETE ON agent_model, agent, agent_halt, agent_eval_run, ai_provenance TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
