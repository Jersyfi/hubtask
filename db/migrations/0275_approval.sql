-- Approvals (M5, UC-SVC-08 to 03): a workflow step waiting for people's yes on one entry, each
-- answer kept as the trail, and a person's delegation while away. An approval keeps its levels
-- as it opened (the step's rule and the people counted then), so changing the workflow later
-- changes no approval in flight. New tables, so expand only (versioning-release.md §4).
-- +goose Up
CREATE TABLE IF NOT EXISTS approval (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id       uuid NOT NULL,
  workflow_key  text NOT NULL,
  step_key      text NOT NULL,
  item_version  integer NOT NULL,
  levels        jsonb NOT NULL,
  decline_to    text NOT NULL DEFAULT '',
  due_hours     integer NOT NULL DEFAULT 0 CHECK (due_hours BETWEEN 0 AND 2160),
  current_level smallint NOT NULL DEFAULT 0 CHECK (current_level BETWEEN 0 AND 4),
  state         text NOT NULL CHECK (state IN ('PENDING', 'APPROVED', 'DECLINED', 'WITHDRAWN')),
  requested_by  uuid NOT NULL,
  requested_at  timestamptz NOT NULL,
  level_since   timestamptz NOT NULL,
  decided_at    timestamptz,
  version       integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT approval_item_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
-- One approval waits on an entry at a time.
CREATE UNIQUE INDEX IF NOT EXISTS approval_pending_uq ON approval (tenant_id, item_id) WHERE state = 'PENDING';
CREATE INDEX IF NOT EXISTS approval_item_idx ON approval (tenant_id, item_id, requested_at DESC);

CREATE TABLE IF NOT EXISTS approval_decision (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  approval_id   uuid NOT NULL,
  level         smallint NOT NULL CHECK (level BETWEEN 0 AND 4),
  approver_id   uuid NOT NULL,
  on_behalf_of  uuid,
  verdict       text NOT NULL CHECK (verdict IN ('APPROVE', 'DECLINE')),
  reason        text NOT NULL DEFAULT '',
  item_version  integer NOT NULL,
  decided_at    timestamptz NOT NULL,
  UNIQUE (tenant_id, id),
  CONSTRAINT approval_decision_approval_fkey
    FOREIGN KEY (tenant_id, approval_id) REFERENCES approval (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT approval_decision_once UNIQUE (tenant_id, approval_id, level, approver_id)
);

CREATE TABLE IF NOT EXISTS approval_delegation (
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id  uuid NOT NULL,
  delegate_id uuid NOT NULL CHECK (delegate_id <> account_id),
  starts_at   timestamptz NOT NULL,
  ends_at     timestamptz NOT NULL CHECK (ends_at > starts_at),
  PRIMARY KEY (tenant_id, account_id)
);

ALTER TABLE approval ENABLE ROW LEVEL SECURITY;
ALTER TABLE approval FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON approval;
CREATE POLICY tenant_isolation ON approval
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON approval TO hubtask_app;
ALTER TABLE approval_decision ENABLE ROW LEVEL SECURITY;
ALTER TABLE approval_decision FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON approval_decision;
CREATE POLICY tenant_isolation ON approval_decision
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON approval_decision TO hubtask_app;
ALTER TABLE approval_delegation ENABLE ROW LEVEL SECURITY;
ALTER TABLE approval_delegation FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON approval_delegation;
CREATE POLICY tenant_isolation ON approval_delegation
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON approval_delegation TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
