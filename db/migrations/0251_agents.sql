-- Governed agents (M9, F-12): the models a workspace registers for its agents, the agents with
-- their policy, budget and lifecycle, the workspace's kill switch, the acts an agent attempted
-- (a ledger for metering, never their input or answer - rule 10), the acts held for a person's
-- approval with their input, the evaluation sets and runs, and the provenance of AI values a
-- person accepted (ai-first.md §2.1, AI-2). New tables only, so expand only (rule 12).
--
-- An agent names its account and its people without a foreign key, as the time module's rows do.
-- A model in use is not deleted (no delete is granted); an agent is retired, never deleted. The
-- ledger's and the held acts' retention is the owner's decision (accepted.md): no policy deletes
-- them here.
-- +goose Up
CREATE TABLE IF NOT EXISTS agent_model (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key            text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_-]{0,63}$'),
  name           text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  source         text NOT NULL CHECK (source IN ('OWN', 'OFFERED')),
  offered_id     uuid,
  model          text NOT NULL CHECK (char_length(model) BETWEEN 1 AND 200),
  region         text NOT NULL CHECK (region IN ('ANY', 'ADEQUACY', 'EEA', 'SELF_HOSTED')),
  retention_days integer CHECK (retention_days BETWEEN 0 AND 3650),
  consent_by     uuid NOT NULL,
  consent_at     timestamptz NOT NULL,
  revision       integer NOT NULL DEFAULT 1 CHECK (revision >= 1),
  retired        boolean NOT NULL DEFAULT false,
  created_at     timestamptz NOT NULL,
  updated_at     timestamptz NOT NULL,
  version        integer NOT NULL DEFAULT 1,
  CHECK ((source = 'OFFERED') = (offered_id IS NOT NULL)),
  UNIQUE (tenant_id, key),
  UNIQUE (tenant_id, id)
);
ALTER TABLE agent_model ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_model FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON agent_model;
CREATE POLICY tenant_isolation ON agent_model
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE ON agent_model TO hubtask_app;

CREATE TABLE IF NOT EXISTS agent (
  id                      uuid PRIMARY KEY,
  tenant_id               uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id              uuid NOT NULL,
  name                    text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  purpose                 text NOT NULL CHECK (char_length(purpose) BETWEEN 1 AND 2000),
  kind                    text NOT NULL CHECK (kind IN ('TICKET_TRIAGE', 'CI_SUGGESTION', 'DUPLICATE_REVIEW', 'KNOWLEDGE_DRAFT', 'EXTERNAL')),
  owner_id                uuid NOT NULL,
  risk_class              text NOT NULL CHECK (risk_class IN ('MINIMAL', 'LIMITED')),
  scopes                  text[] NOT NULL DEFAULT '{}' CHECK (cardinality(scopes) <= 64),
  model_id                uuid,
  policy_reversible       text NOT NULL CHECK (policy_reversible IN ('ALLOW', 'NOTIFY', 'APPROVAL', 'NEVER')),
  policy_irreversible     text NOT NULL CHECK (policy_irreversible IN ('APPROVAL', 'NEVER')),
  tokens_per_day          bigint NOT NULL CHECK (tokens_per_day >= 0),
  actions_per_hour        integer NOT NULL CHECK (actions_per_hour >= 0),
  concurrent_runs         integer NOT NULL CHECK (concurrent_runs >= 0),
  min_accuracy            smallint NOT NULL DEFAULT 0 CHECK (min_accuracy BETWEEN 0 AND 100),
  status                  text NOT NULL CHECK (status IN ('DRAFT', 'APPROVED', 'ACTIVE', 'SUSPENDED', 'RETIRED')),
  approved_by             uuid,
  approved_at             timestamptz,
  approved_model_revision integer NOT NULL DEFAULT 0,
  suspended_by            uuid,
  suspended_at            timestamptz,
  created_by              uuid NOT NULL,
  created_at              timestamptz NOT NULL,
  updated_at              timestamptz NOT NULL,
  version                 integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, account_id),
  UNIQUE (tenant_id, id),
  CONSTRAINT agent_model_fkey FOREIGN KEY (tenant_id, model_id) REFERENCES agent_model (tenant_id, id)
);
ALTER TABLE agent ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON agent;
CREATE POLICY tenant_isolation ON agent
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE ON agent TO hubtask_app;

CREATE TABLE IF NOT EXISTS agent_halt (
  id        uuid PRIMARY KEY,
  tenant_id uuid NOT NULL UNIQUE REFERENCES tenant(id) ON DELETE CASCADE,
  halted    boolean NOT NULL,
  set_by    uuid NOT NULL,
  set_at    timestamptz NOT NULL
);
ALTER TABLE agent_halt ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_halt FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON agent_halt;
CREATE POLICY tenant_isolation ON agent_halt
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE ON agent_halt TO hubtask_app;

CREATE TABLE IF NOT EXISTS agent_request (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  agent_id   uuid NOT NULL,
  use_case   text NOT NULL CHECK (char_length(use_case) BETWEEN 1 AND 120),
  class      text NOT NULL CHECK (class IN ('REVERSIBLE', 'IRREVERSIBLE')),
  input      jsonb NOT NULL,
  status     text NOT NULL CHECK (status IN ('PENDING', 'APPROVED', 'DECLINED', 'FAILED')),
  decided_by uuid,
  decided_at timestamptz,
  code       text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL,
  version    integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT agent_request_agent_fkey FOREIGN KEY (tenant_id, agent_id) REFERENCES agent (tenant_id, id)
);
CREATE INDEX IF NOT EXISTS agent_request_pending_idx ON agent_request (tenant_id, status, created_at);
ALTER TABLE agent_request ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_request FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON agent_request;
CREATE POLICY tenant_isolation ON agent_request
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE ON agent_request TO hubtask_app;

CREATE TABLE IF NOT EXISTS agent_action (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  agent_id   uuid NOT NULL,
  use_case   text NOT NULL CHECK (char_length(use_case) BETWEEN 1 AND 120),
  class      text NOT NULL CHECK (class IN ('READ', 'REVERSIBLE', 'IRREVERSIBLE')),
  decision   text NOT NULL CHECK (decision IN ('ALLOWED', 'NOTIFIED', 'HELD', 'REFUSED')),
  code       text NOT NULL DEFAULT '',
  tokens     bigint NOT NULL DEFAULT 0 CHECK (tokens >= 0),
  request_id uuid,
  at         timestamptz NOT NULL,
  CONSTRAINT agent_action_agent_fkey FOREIGN KEY (tenant_id, agent_id) REFERENCES agent (tenant_id, id)
);
CREATE INDEX IF NOT EXISTS agent_action_agent_idx ON agent_action (tenant_id, agent_id, at);
ALTER TABLE agent_action ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_action FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON agent_action;
CREATE POLICY tenant_isolation ON agent_action
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT ON agent_action TO hubtask_app;

CREATE TABLE IF NOT EXISTS agent_eval_case (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  agent_id   uuid NOT NULL,
  title      text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 500),
  notes      text NOT NULL DEFAULT '' CHECK (char_length(notes) <= 20000),
  expected   jsonb NOT NULL,
  created_by uuid NOT NULL,
  created_at timestamptz NOT NULL,
  CONSTRAINT agent_eval_case_agent_fkey FOREIGN KEY (tenant_id, agent_id) REFERENCES agent (tenant_id, id)
);
CREATE INDEX IF NOT EXISTS agent_eval_case_agent_idx ON agent_eval_case (tenant_id, agent_id, created_at);
ALTER TABLE agent_eval_case ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_eval_case FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON agent_eval_case;
CREATE POLICY tenant_isolation ON agent_eval_case
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON agent_eval_case TO hubtask_app;

CREATE TABLE IF NOT EXISTS agent_eval_run (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  agent_id       uuid NOT NULL,
  model_revision integer NOT NULL,
  cases          integer NOT NULL CHECK (cases >= 0),
  passed         integer NOT NULL CHECK (passed BETWEEN 0 AND cases),
  run_by         uuid NOT NULL,
  at             timestamptz NOT NULL,
  CONSTRAINT agent_eval_run_agent_fkey FOREIGN KEY (tenant_id, agent_id) REFERENCES agent (tenant_id, id)
);
CREATE INDEX IF NOT EXISTS agent_eval_run_agent_idx ON agent_eval_run (tenant_id, agent_id, at);
ALTER TABLE agent_eval_run ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_eval_run FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON agent_eval_run;
CREATE POLICY tenant_isolation ON agent_eval_run
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT ON agent_eval_run TO hubtask_app;

CREATE TABLE IF NOT EXISTS ai_provenance (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  target_type    text NOT NULL CHECK (char_length(target_type) BETWEEN 1 AND 64),
  target_id      uuid NOT NULL,
  fields         text[] NOT NULL DEFAULT '{}',
  suggestion_id  uuid,
  agent_id       uuid,
  model          text NOT NULL CHECK (char_length(model) BETWEEN 1 AND 200),
  prompt_id      text NOT NULL DEFAULT '',
  prompt_version text NOT NULL DEFAULT '',
  accepted_by    uuid NOT NULL,
  accepted_at    timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS ai_provenance_target_idx ON ai_provenance (tenant_id, target_type, target_id, accepted_at);
ALTER TABLE ai_provenance ENABLE ROW LEVEL SECURITY;
ALTER TABLE ai_provenance FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON ai_provenance;
CREATE POLICY tenant_isolation ON ai_provenance
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT ON ai_provenance TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
