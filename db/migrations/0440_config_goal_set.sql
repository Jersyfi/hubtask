-- A template's goal sets (configuration.md §6): the mark that a goal period was brought by the
-- workspace's document - a quarter's objectives with their key results, or a period alone - under
-- its key, so the document reads it back and never brings it twice. Two sets of the same days share
-- one period. The period and its goals are ordinary ones; the mark leaves with the period.
--
-- Expand only (rule 12): a new table nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS config_goal_set (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key        text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  period_id  uuid NOT NULL,
  created_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, key),
  CONSTRAINT config_goal_set_period_fkey FOREIGN KEY (tenant_id, period_id)
    REFERENCES goal_period (tenant_id, id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED
);

ALTER TABLE config_goal_set ENABLE ROW LEVEL SECURITY;
ALTER TABLE config_goal_set FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON config_goal_set;
CREATE POLICY tenant_isolation ON config_goal_set
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON config_goal_set TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
