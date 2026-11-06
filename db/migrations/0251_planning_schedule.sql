-- Planning (M2, PLN-02 … PLN-05): a dependency's timing, an entry's scheduling mode, a plan's
-- baselines. Two columns with constant defaults and three new tables, so expand only
-- (versioning-release.md §4): every link drawn before stays finish-to-start without a lag, every
-- entry manual - nothing moves that nobody asked to move.
--
-- * `entry_link.dependency_kind` and `lag_days` say how a waiting link's successor hangs on its
--   predecessor; they are read only for a link type one end waits on, and are the only columns of
--   a link that change after it is drawn.
-- * `entry_schedule` is an entry's mode (MANUAL kept where a person put it, AUTOMATIC placed by its
--   predecessors) and whether it is a milestone. No row is MANUAL and no milestone.
-- * `plan_baseline` and `plan_baseline_entry` are the dates of a hub's or a collection's entries as
--   they were agreed. A baseline is taken and deleted, never changed. Its entries name an entry
--   without a foreign key: an entry deleted since is what the comparison shows as gone.
-- +goose Up
ALTER TABLE entry_link ADD COLUMN IF NOT EXISTS dependency_kind text NOT NULL DEFAULT 'FS'
  CHECK (dependency_kind IN ('FS', 'SS', 'FF', 'SF'));
ALTER TABLE entry_link ADD COLUMN IF NOT EXISTS lag_days integer NOT NULL DEFAULT 0
  CHECK (lag_days BETWEEN -999 AND 999);
GRANT UPDATE (dependency_kind, lag_days) ON entry_link TO hubtask_app;

CREATE TABLE IF NOT EXISTS entry_schedule (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  entry_id   uuid NOT NULL,
  mode       text NOT NULL CHECK (mode IN ('MANUAL', 'AUTOMATIC')),
  milestone  boolean NOT NULL DEFAULT false,
  updated_at timestamptz NOT NULL,
  version    integer NOT NULL DEFAULT 1,
  PRIMARY KEY (tenant_id, entry_id),
  CONSTRAINT entry_schedule_entry_fkey
    FOREIGN KEY (tenant_id, entry_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
ALTER TABLE entry_schedule ENABLE ROW LEVEL SECURITY;
ALTER TABLE entry_schedule FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON entry_schedule;
CREATE POLICY tenant_isolation ON entry_schedule
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON entry_schedule TO hubtask_app;

CREATE TABLE IF NOT EXISTS plan_baseline (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  plan_id     uuid NOT NULL,
  name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  taken_at    timestamptz NOT NULL,
  taken_by    uuid,
  entry_count integer NOT NULL DEFAULT 0 CHECK (entry_count BETWEEN 0 AND 10000),
  UNIQUE (tenant_id, id),
  CONSTRAINT plan_baseline_plan_fkey
    FOREIGN KEY (tenant_id, plan_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS plan_baseline_plan_idx ON plan_baseline (tenant_id, plan_id, taken_at);
ALTER TABLE plan_baseline ENABLE ROW LEVEL SECURITY;
ALTER TABLE plan_baseline FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON plan_baseline;
CREATE POLICY tenant_isolation ON plan_baseline
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON plan_baseline TO hubtask_app;

CREATE TABLE IF NOT EXISTS plan_baseline_entry (
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  baseline_id uuid NOT NULL,
  entry_id    uuid NOT NULL,
  start_at    timestamptz,
  due_at      timestamptz,
  PRIMARY KEY (tenant_id, baseline_id, entry_id),
  CONSTRAINT plan_baseline_entry_baseline_fkey
    FOREIGN KEY (tenant_id, baseline_id) REFERENCES plan_baseline (tenant_id, id) ON DELETE CASCADE
);
ALTER TABLE plan_baseline_entry ENABLE ROW LEVEL SECURITY;
ALTER TABLE plan_baseline_entry FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON plan_baseline_entry;
CREATE POLICY tenant_isolation ON plan_baseline_entry
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON plan_baseline_entry TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
