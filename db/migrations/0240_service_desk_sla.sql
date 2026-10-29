-- Business calendars and SLAs (M5, UC-SVC-01, UC-SVC-02). Three tables, all new, so expand only
-- (versioning-release.md §4):
--
-- * `business_calendar` is a workspace's calendar under a key: working spans per weekday,
--   holidays and a zone, in the shape a durable timer keeps its copy in (`calendar`, 0229).
-- * `sla_policy` is a promise for entries of some types: its goals (first response, resolution)
--   with targets per priority, the statuses that pause them and a warning share, on a calendar or
--   around the clock. A calendar a policy names cannot be deleted (foreign key on the key).
-- * `entry_sla` is what became of each goal on one entry - its target, when it was at risk, met
--   or breached. The clock is a durable timer (not archived, §8.4); this row is the record the
--   archive carries and a report reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS business_calendar (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key         text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  spec        jsonb NOT NULL,
  created_at  timestamptz NOT NULL,
  updated_at  timestamptz NOT NULL,
  version     integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT business_calendar_key_uq UNIQUE (tenant_id, key)
);
ALTER TABLE business_calendar ENABLE ROW LEVEL SECURITY;
ALTER TABLE business_calendar FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON business_calendar;
CREATE POLICY tenant_isolation ON business_calendar
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON business_calendar TO hubtask_app;

CREATE TABLE IF NOT EXISTS sla_policy (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key            text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  name           text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  calendar_key   text CHECK (calendar_key ~ '^[a-z][a-z0-9_]{0,49}$'),
  entry_types    text[] NOT NULL,
  priority_field text NOT NULL DEFAULT '',
  goals          jsonb NOT NULL,
  position       integer NOT NULL DEFAULT 0,
  enabled        boolean NOT NULL DEFAULT true,
  created_at     timestamptz NOT NULL,
  updated_at     timestamptz NOT NULL,
  version        integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT sla_policy_key_uq UNIQUE (tenant_id, key),
  CONSTRAINT sla_policy_calendar_fkey
    FOREIGN KEY (tenant_id, calendar_key) REFERENCES business_calendar (tenant_id, key)
);
ALTER TABLE sla_policy ENABLE ROW LEVEL SECURITY;
ALTER TABLE sla_policy FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON sla_policy;
CREATE POLICY tenant_isolation ON sla_policy
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON sla_policy TO hubtask_app;

CREATE TABLE IF NOT EXISTS entry_sla (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id        uuid NOT NULL,
  goal           text NOT NULL CHECK (goal IN ('FIRST_RESPONSE', 'RESOLUTION')),
  policy_key     text NOT NULL,
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
  CONSTRAINT entry_sla_goal_uq UNIQUE (tenant_id, item_id, goal),
  CONSTRAINT entry_sla_item_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE,
  CHECK ((standing = 'MET') = (met_at IS NOT NULL)),
  CHECK (standing <> 'BREACHED' OR breached_at IS NOT NULL)
);
-- A queue sorted by the next breach, and a report by standing.
CREATE INDEX IF NOT EXISTS entry_sla_due_idx ON entry_sla (tenant_id, due_at) WHERE standing = 'RUNNING';
CREATE INDEX IF NOT EXISTS entry_sla_standing_idx ON entry_sla (tenant_id, policy_key, standing);
ALTER TABLE entry_sla ENABLE ROW LEVEL SECURITY;
ALTER TABLE entry_sla FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON entry_sla;
CREATE POLICY tenant_isolation ON entry_sla
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON entry_sla TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
