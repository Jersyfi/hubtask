-- Time (M7, UC-TIM-01 … 05): kinds of work, time records, running timers, a person's weeks and
-- the workspace's time policy. Five new tables, so expand only (versioning-release.md §4).
--
-- * `time_work_type` is a kind of work a workspace records time as, by a stable key.
-- * `time_record` is one person's time on one day, on an entry or on none. It names its entry
--   without a foreign key: approved time is accounting's and outlives an entry purged from the
--   trash; a report reads such a record as time on no entry. A correction is signed and names the
--   approved record it corrects.
-- * `time_timer` is a person's running clock, at most one per person.
-- * `timesheet` is a person's week from Monday once it was handed in; a week without a row is open.
-- * `time_policy` is the workspace's locked period and what a lead sees of other people's time.
-- +goose Up
CREATE TABLE IF NOT EXISTS time_work_type (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key        text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,39}$'),
  name       text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
  billable   boolean NOT NULL DEFAULT false,
  archived   boolean NOT NULL DEFAULT false,
  position   integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  version    integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, key)
);
ALTER TABLE time_work_type ENABLE ROW LEVEL SECURITY;
ALTER TABLE time_work_type FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON time_work_type;
CREATE POLICY tenant_isolation ON time_work_type
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON time_work_type TO hubtask_app;

CREATE TABLE IF NOT EXISTS time_record (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id uuid NOT NULL,
  entry_id   uuid,
  work_type  text NOT NULL DEFAULT '',
  day        date NOT NULL,
  started_at timestamptz,
  minutes    integer NOT NULL CHECK (minutes <> 0 AND minutes BETWEEN -1440 AND 1440),
  note       text NOT NULL DEFAULT '' CHECK (char_length(note) <= 2000),
  billable   boolean NOT NULL DEFAULT false,
  source     text NOT NULL CHECK (source IN ('MANUAL', 'TIMER', 'COPY', 'CORRECTION')),
  corrects   uuid,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  version    integer NOT NULL DEFAULT 1,
  CHECK ((source = 'CORRECTION') = (corrects IS NOT NULL)),
  CHECK (source = 'CORRECTION' OR minutes > 0)
);
CREATE INDEX IF NOT EXISTS time_record_person_idx ON time_record (tenant_id, account_id, day);
CREATE INDEX IF NOT EXISTS time_record_day_idx ON time_record (tenant_id, day);
CREATE INDEX IF NOT EXISTS time_record_entry_idx ON time_record (tenant_id, entry_id) WHERE entry_id IS NOT NULL;
ALTER TABLE time_record ENABLE ROW LEVEL SECURITY;
ALTER TABLE time_record FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON time_record;
CREATE POLICY tenant_isolation ON time_record
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON time_record TO hubtask_app;

CREATE TABLE IF NOT EXISTS time_timer (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id uuid NOT NULL,
  entry_id   uuid,
  work_type  text NOT NULL DEFAULT '',
  note       text NOT NULL DEFAULT '' CHECK (char_length(note) <= 2000),
  billable   boolean NOT NULL DEFAULT false,
  started_at timestamptz NOT NULL,
  version    integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, account_id)
);
ALTER TABLE time_timer ENABLE ROW LEVEL SECURITY;
ALTER TABLE time_timer FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON time_timer;
CREATE POLICY tenant_isolation ON time_timer
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON time_timer TO hubtask_app;

CREATE TABLE IF NOT EXISTS timesheet (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id   uuid NOT NULL,
  week_start   date NOT NULL CHECK (extract(isodow FROM week_start) = 1),
  state        text NOT NULL CHECK (state IN ('OPEN', 'SUBMITTED', 'APPROVED', 'REJECTED')),
  minutes      integer NOT NULL DEFAULT 0,
  submitted_at timestamptz,
  decided_at   timestamptz,
  decided_by   uuid,
  reason       text NOT NULL DEFAULT '' CHECK (char_length(reason) <= 500),
  updated_at   timestamptz NOT NULL,
  version      integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, account_id, week_start)
);
CREATE INDEX IF NOT EXISTS timesheet_state_idx ON timesheet (tenant_id, state, week_start);
ALTER TABLE timesheet ENABLE ROW LEVEL SECURITY;
ALTER TABLE timesheet FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON timesheet;
CREATE POLICY tenant_isolation ON timesheet
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON timesheet TO hubtask_app;

CREATE TABLE IF NOT EXISTS time_policy (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  locked_through date,
  visibility     text NOT NULL DEFAULT 'FULL' CHECK (visibility IN ('FULL', 'AGGREGATED')),
  updated_at     timestamptz NOT NULL,
  updated_by     uuid,
  version        integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id)
);
ALTER TABLE time_policy ENABLE ROW LEVEL SECURITY;
ALTER TABLE time_policy FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON time_policy;
CREATE POLICY tenant_isolation ON time_policy
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON time_policy TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
