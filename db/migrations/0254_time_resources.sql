-- Time's resources and costs (M7, UC-TIM-06 … 08): a person's capacity, their absences, the
-- hourly cost rates of people and roles, and a hub's or collection's budget. Four new tables, so
-- expand only (versioning-release.md §4).
--
-- * `time_capacity` is a person's minutes per weekday from a day on, until their next one.
-- * `time_absence` is a span of days a person is away, wholly or half of each day, by a kind -
--   never a reason: a shared calendar must not tell an illness (GDPR Art. 9).
-- * `time_cost_rate` is an hour's cost of a person or of a role, from a day to a day or on.
-- * `time_budget` is a plan's budget in minutes, in money of one currency, or both.
-- +goose Up
CREATE TABLE IF NOT EXISTS time_capacity (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id   uuid NOT NULL,
  from_day     date NOT NULL,
  week_minutes integer[] NOT NULL CHECK (cardinality(week_minutes) = 7
    AND 0 <= ALL (week_minutes) AND 1440 >= ALL (week_minutes)),
  updated_at   timestamptz NOT NULL,
  updated_by   uuid,
  version      integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, account_id, from_day)
);
ALTER TABLE time_capacity ENABLE ROW LEVEL SECURITY;
ALTER TABLE time_capacity FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON time_capacity;
CREATE POLICY tenant_isolation ON time_capacity
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON time_capacity TO hubtask_app;

CREATE TABLE IF NOT EXISTS time_absence (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id uuid NOT NULL,
  from_day   date NOT NULL,
  to_day     date NOT NULL,
  kind       text NOT NULL CHECK (kind IN ('LEAVE', 'TRAINING', 'OTHER')),
  half       boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL,
  created_by uuid,
  updated_at timestamptz NOT NULL,
  version    integer NOT NULL DEFAULT 1,
  CHECK (to_day >= from_day AND to_day - from_day < 366)
);
CREATE INDEX IF NOT EXISTS time_absence_person_idx ON time_absence (tenant_id, account_id, from_day);
CREATE INDEX IF NOT EXISTS time_absence_day_idx ON time_absence (tenant_id, to_day, from_day);
ALTER TABLE time_absence ENABLE ROW LEVEL SECURITY;
ALTER TABLE time_absence FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON time_absence;
CREATE POLICY tenant_isolation ON time_absence
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON time_absence TO hubtask_app;

CREATE TABLE IF NOT EXISTS time_cost_rate (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id uuid,
  role       text CHECK (char_length(role) BETWEEN 1 AND 64),
  hour_minor bigint NOT NULL CHECK (hour_minor BETWEEN 0 AND 1000000000),
  currency   text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  from_day   date NOT NULL,
  to_day     date,
  updated_at timestamptz NOT NULL,
  updated_by uuid,
  version    integer NOT NULL DEFAULT 1,
  CHECK ((account_id IS NULL) <> (role IS NULL)),
  CHECK (to_day IS NULL OR to_day >= from_day)
);
CREATE INDEX IF NOT EXISTS time_cost_rate_subject_idx ON time_cost_rate (tenant_id, account_id, role, from_day);
ALTER TABLE time_cost_rate ENABLE ROW LEVEL SECURITY;
ALTER TABLE time_cost_rate FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON time_cost_rate;
CREATE POLICY tenant_isolation ON time_cost_rate
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON time_cost_rate TO hubtask_app;

CREATE TABLE IF NOT EXISTS time_budget (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  plan_id      uuid NOT NULL,
  minutes      bigint NOT NULL DEFAULT 0 CHECK (minutes BETWEEN 0 AND 60000000),
  amount_minor bigint NOT NULL DEFAULT 0 CHECK (amount_minor BETWEEN 0 AND 10000000000000),
  currency     text NOT NULL DEFAULT '' CHECK (currency = '' OR currency ~ '^[A-Z]{3}$'),
  updated_at   timestamptz NOT NULL,
  updated_by   uuid,
  version      integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, plan_id),
  CHECK (amount_minor = 0 OR currency <> ''),
  CONSTRAINT time_budget_plan_fkey
    FOREIGN KEY (tenant_id, plan_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
ALTER TABLE time_budget ENABLE ROW LEVEL SECURITY;
ALTER TABLE time_budget FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON time_budget;
CREATE POLICY tenant_isolation ON time_budget
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON time_budget TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
