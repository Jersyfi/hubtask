-- Planning (M2, PLN-06, PLN-07): sprints, the entries in them, and an entry's estimate. Three new
-- tables, so expand only (versioning-release.md §4).
--
-- * `sprint` is a time box of one collection with a goal; its commitment is recorded at the start
--   and its result at the close. At most one runs per collection at a time.
-- * `sprint_entry` is an entry's membership: when it joined and left, whether it was committed,
--   and what became of it at the close. An entry is in at most one open membership; a closed
--   sprint keeps its rows as its history.
-- * `entry_estimate` is how big an entry is, in points, hours or a T-shirt size (with the points it
--   counts as), so that velocity adds up without guessing a custom field.
-- +goose Up
CREATE TABLE IF NOT EXISTS sprint (
  id              uuid PRIMARY KEY,
  tenant_id       uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  collection_id   uuid NOT NULL,
  name            text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  goal            text NOT NULL DEFAULT '' CHECK (char_length(goal) <= 500),
  start_on        date NOT NULL,
  end_on          date NOT NULL,
  state           text NOT NULL CHECK (state IN ('PLANNED', 'ACTIVE', 'CLOSED')),
  unit            text NOT NULL DEFAULT 'POINTS' CHECK (unit IN ('POINTS', 'HOURS')),
  committed_value double precision NOT NULL DEFAULT 0,
  committed_count integer NOT NULL DEFAULT 0,
  completed_value double precision NOT NULL DEFAULT 0,
  completed_count integer NOT NULL DEFAULT 0,
  started_at      timestamptz,
  closed_at       timestamptz,
  created_at      timestamptz NOT NULL,
  created_by      uuid,
  updated_at      timestamptz NOT NULL,
  version         integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CHECK (end_on >= start_on AND end_on - start_on < 92),
  CONSTRAINT sprint_collection_fkey
    FOREIGN KEY (tenant_id, collection_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS sprint_collection_idx ON sprint (tenant_id, collection_id, start_on);
CREATE UNIQUE INDEX IF NOT EXISTS sprint_one_active_idx ON sprint (tenant_id, collection_id)
  WHERE state = 'ACTIVE';
ALTER TABLE sprint ENABLE ROW LEVEL SECURITY;
ALTER TABLE sprint FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON sprint;
CREATE POLICY tenant_isolation ON sprint
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON sprint TO hubtask_app;

CREATE TABLE IF NOT EXISTS sprint_entry (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  sprint_id  uuid NOT NULL,
  entry_id   uuid NOT NULL,
  added_at   timestamptz NOT NULL,
  removed_at timestamptz,
  committed  boolean NOT NULL DEFAULT false,
  outcome    text CHECK (outcome IN ('DONE', 'CARRIED', 'RETURNED')),
  PRIMARY KEY (tenant_id, sprint_id, entry_id),
  CONSTRAINT sprint_entry_sprint_fkey
    FOREIGN KEY (tenant_id, sprint_id) REFERENCES sprint (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT sprint_entry_entry_fkey
    FOREIGN KEY (tenant_id, entry_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS sprint_entry_one_open_idx ON sprint_entry (tenant_id, entry_id)
  WHERE removed_at IS NULL AND outcome IS NULL;
ALTER TABLE sprint_entry ENABLE ROW LEVEL SECURITY;
ALTER TABLE sprint_entry FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON sprint_entry;
CREATE POLICY tenant_isolation ON sprint_entry
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON sprint_entry TO hubtask_app;

CREATE TABLE IF NOT EXISTS entry_estimate (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  entry_id   uuid NOT NULL,
  unit       text NOT NULL CHECK (unit IN ('POINTS', 'HOURS', 'SIZE')),
  value      double precision NOT NULL CHECK (value >= 0 AND value <= 10000),
  size       text CHECK (size IN ('XS', 'S', 'M', 'L', 'XL', 'XXL')),
  updated_at timestamptz NOT NULL,
  updated_by uuid,
  version    integer NOT NULL DEFAULT 1,
  PRIMARY KEY (tenant_id, entry_id),
  CHECK ((unit = 'SIZE') = (size IS NOT NULL)),
  CONSTRAINT entry_estimate_entry_fkey
    FOREIGN KEY (tenant_id, entry_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
ALTER TABLE entry_estimate ENABLE ROW LEVEL SECURITY;
ALTER TABLE entry_estimate FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON entry_estimate;
CREATE POLICY tenant_isolation ON entry_estimate
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON entry_estimate TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
