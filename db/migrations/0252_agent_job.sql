-- A shipped agent's jobs (M9, G11): one run on one target - an entry, or a hub's inventory -,
-- what it proposed and the tokens it spent, never what it read or what the model answered (rule
-- 10). A running job counts against its agent's concurrent runs for its lease; one whose worker
-- died stops counting by itself. A new table only, so expand only (rule 12). Its retention is the
-- owner's decision with the ledger's (accepted.md): no policy deletes it here.
-- +goose Up
CREATE TABLE IF NOT EXISTS agent_job (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  agent_id    uuid NOT NULL,
  target_type text NOT NULL CHECK (target_type IN ('ENTRY', 'HUB')),
  target_id   uuid NOT NULL,
  status      text NOT NULL CHECK (status IN ('RUNNING', 'DONE', 'FAILED')),
  code        text NOT NULL DEFAULT '',
  proposals   integer NOT NULL DEFAULT 0 CHECK (proposals >= 0),
  tokens      bigint NOT NULL DEFAULT 0 CHECK (tokens >= 0),
  started_by  uuid,
  started_at  timestamptz NOT NULL,
  ended_at    timestamptz,
  CHECK ((status = 'RUNNING') = (ended_at IS NULL)),
  CONSTRAINT agent_job_agent_fkey FOREIGN KEY (tenant_id, agent_id) REFERENCES agent (tenant_id, id)
);
CREATE INDEX IF NOT EXISTS agent_job_agent_idx ON agent_job (tenant_id, agent_id, started_at);
ALTER TABLE agent_job ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_job FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON agent_job;
CREATE POLICY tenant_isolation ON agent_job
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE ON agent_job TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
