-- Operational level agreements (UC-SVC-02 checks 10 to 12; products.md M5 "SLAs/OLAs"): an SLA
-- policy that names an assignee group is the team's internal promise, armed while the entry is on
-- a member of that group and measured beside the requester's.
--
-- * `sla_policy.assignee_group_id` names the group; NULL is the requester's promise, as every
--   policy was before. No foreign key: a deleted group leaves the policy measuring nobody rather
--   than turning it into a requester's promise (SET NULL) or deleting it (CASCADE).
-- * `entry_ola` is what became of each goal of each OLA on one entry, keyed by entry, policy and
--   goal - its own table, because `entry_sla` is one row per entry and goal and an older binary
--   upserts on that key; it never reads this one.
-- * The OLA's clocks are durable timers of their own purpose whose subject is the record.
--
-- Expand only (rule 12): a nullable column, a new table, a widened check (NOT VALID, then
-- VALIDATE, so checking the rows stops no writes). An older binary still running takes a policy
-- defined with a group in that window for a requester's promise; it measures more, never less.
-- +goose Up
ALTER TABLE sla_policy ADD COLUMN IF NOT EXISTS assignee_group_id uuid;

CREATE TABLE IF NOT EXISTS entry_ola (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id        uuid NOT NULL,
  policy_key     text NOT NULL,
  group_id       uuid NOT NULL,
  goal           text NOT NULL CHECK (goal IN ('FIRST_RESPONSE', 'RESOLUTION')),
  target_seconds bigint NOT NULL CHECK (target_seconds > 0),
  standing       text NOT NULL CHECK (standing IN ('RUNNING', 'PAUSED', 'MET', 'BREACHED')),
  armed_at       timestamptz NOT NULL,
  due_at         timestamptz,
  at_risk_at     timestamptz,
  met_at         timestamptz,
  breached_at    timestamptz,
  updated_at     timestamptz NOT NULL,
  version        integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT entry_ola_goal_uq UNIQUE (tenant_id, item_id, policy_key, goal),
  CONSTRAINT entry_ola_item_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE,
  CHECK ((standing = 'MET') = (met_at IS NOT NULL)),
  CHECK (standing <> 'BREACHED' OR breached_at IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS entry_ola_standing_idx ON entry_ola (tenant_id, policy_key, standing);
ALTER TABLE entry_ola ENABLE ROW LEVEL SECURITY;
ALTER TABLE entry_ola FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON entry_ola;
CREATE POLICY tenant_isolation ON entry_ola
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON entry_ola TO hubtask_app;

ALTER TABLE durable_timer DROP CONSTRAINT IF EXISTS durable_timer_purpose_check;
ALTER TABLE durable_timer ADD CONSTRAINT durable_timer_purpose_check
  CHECK (purpose IN ('SLA', 'APPROVAL', 'EFFECTIVENESS_CHECK', 'PAGE_REVIEW', 'OLA')) NOT VALID;
ALTER TABLE durable_timer VALIDATE CONSTRAINT durable_timer_purpose_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
