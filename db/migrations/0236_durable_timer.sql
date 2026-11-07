-- Durable timers on business calendars (F-10, domain-model.md §3.7): the clock under an SLA, an
-- approval's deadline and a change's effectiveness check, as a row a worker fires rather than a
-- process that sleeps.
--
-- `calendar` is the copy of the calendar the timer was armed on (zone, weekly spans, holidays), so
-- a calendar changed later moves no clock already running. A running timer has `running_since`
-- and `due_at`; a paused one keeps what it spent in `consumed_seconds`. One live timer per
-- purpose, subject and name. The subject is polymorphic (an entry, an approval), so no foreign
-- key: the handler of a purpose decides what a timer whose subject is gone means.
--
-- Expand only: a new table nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS durable_timer (
  id               uuid PRIMARY KEY,
  tenant_id        uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  purpose          text NOT NULL CHECK (purpose IN ('SLA', 'APPROVAL', 'EFFECTIVENESS_CHECK')),
  subject_id       uuid NOT NULL,
  name             text NOT NULL CHECK (name ~ '^[a-z][a-z0-9_]{0,49}$'),
  calendar         jsonb NOT NULL,
  budget_seconds   bigint NOT NULL CHECK (budget_seconds > 0),
  consumed_seconds bigint NOT NULL DEFAULT 0 CHECK (consumed_seconds >= 0),
  running_since    timestamptz,
  due_at           timestamptz,
  state            text NOT NULL CHECK (state IN ('RUNNING', 'PAUSED', 'FIRED', 'STOPPED')),
  fired_at         timestamptz,
  created_at       timestamptz NOT NULL,
  updated_at       timestamptz NOT NULL,
  version          integer NOT NULL DEFAULT 1,
  CHECK ((state = 'RUNNING') = (running_since IS NOT NULL)),
  CHECK (state <> 'RUNNING' OR due_at IS NOT NULL),
  CHECK ((state = 'FIRED') = (fired_at IS NOT NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS durable_timer_live_uq
  ON durable_timer (tenant_id, purpose, subject_id, name) WHERE state IN ('RUNNING', 'PAUSED');
-- The firing pass: a tenant's running timers by due moment.
CREATE INDEX IF NOT EXISTS durable_timer_due_idx
  ON durable_timer (tenant_id, due_at) WHERE state = 'RUNNING';

ALTER TABLE durable_timer ENABLE ROW LEVEL SECURITY;
ALTER TABLE durable_timer FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON durable_timer;
CREATE POLICY tenant_isolation ON durable_timer
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON durable_timer TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
