-- Goals and insight (M8, UC-GOL-01 … 03, UC-GOL-04): periods, goals with their key results, the
-- entries linked to a goal, check-ins, portfolios of hubs and collections with status updates, and
-- dashboards. New tables only, so expand only (versioning-release.md §4).
--
-- * `goal_period` is a cycle goals are set for; closing it grades every goal of it.
-- * `goal` is an objective of a period, at the workspace (`hub_id` null) or in a hub, going with
--   its hub. Its key results are a small ordered list that only changes with the goal, so they
--   are one `jsonb` column; `parent_id` carries no foreign key - deleting a goal hands its goals to
--   its own parent in the same transaction.
-- * `goal_entry` links an entry to a goal; its progress counts the share completed.
-- * `goal_check_in` and `portfolio_update` are history: written once, never changed.
-- * `portfolio_item` is a hub or collection of a portfolio, going with its container.
-- * `dashboard` holds its widgets as `jsonb`: their numbers are computed when read, never stored.
-- +goose Up
CREATE TABLE IF NOT EXISTS goal_period (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  name       text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
  start_on   date NOT NULL,
  end_on     date NOT NULL,
  state      text NOT NULL CHECK (state IN ('OPEN', 'CLOSED')),
  closed_at  timestamptz,
  closed_by  uuid,
  created_at timestamptz NOT NULL,
  created_by uuid,
  updated_at timestamptz NOT NULL,
  version    integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CHECK (end_on >= start_on)
);
ALTER TABLE goal_period ENABLE ROW LEVEL SECURITY;
ALTER TABLE goal_period FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON goal_period;
CREATE POLICY tenant_isolation ON goal_period
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON goal_period TO hubtask_app;

CREATE TABLE IF NOT EXISTS goal (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  period_id   uuid NOT NULL,
  hub_id      uuid,
  parent_id   uuid,
  title       text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
  description text NOT NULL DEFAULT '' CHECK (char_length(description) <= 10000),
  owner_id    uuid,
  status      text NOT NULL CHECK (status IN ('NONE', 'ON_TRACK', 'AT_RISK', 'OFF_TRACK', 'DONE', 'DROPPED')),
  confidence  integer NOT NULL DEFAULT -1 CHECK (confidence BETWEEN -1 AND 10),
  key_results jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(key_results) = 'array'),
  grade       double precision CHECK (grade BETWEEN 0 AND 1),
  created_at  timestamptz NOT NULL,
  created_by  uuid,
  updated_at  timestamptz NOT NULL,
  updated_by  uuid,
  version     integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT goal_period_fkey
    FOREIGN KEY (tenant_id, period_id) REFERENCES goal_period (tenant_id, id),
  CONSTRAINT goal_hub_fkey
    FOREIGN KEY (tenant_id, hub_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS goal_period_idx ON goal (tenant_id, period_id);
CREATE INDEX IF NOT EXISTS goal_parent_idx ON goal (tenant_id, parent_id) WHERE parent_id IS NOT NULL;
ALTER TABLE goal ENABLE ROW LEVEL SECURITY;
ALTER TABLE goal FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON goal;
CREATE POLICY tenant_isolation ON goal
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON goal TO hubtask_app;

CREATE TABLE IF NOT EXISTS goal_entry (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  goal_id    uuid NOT NULL,
  entry_id   uuid NOT NULL,
  created_at timestamptz NOT NULL,
  created_by uuid,
  PRIMARY KEY (tenant_id, goal_id, entry_id),
  CONSTRAINT goal_entry_goal_fkey
    FOREIGN KEY (tenant_id, goal_id) REFERENCES goal (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT goal_entry_entry_fkey
    FOREIGN KEY (tenant_id, entry_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS goal_entry_entry_idx ON goal_entry (tenant_id, entry_id);
ALTER TABLE goal_entry ENABLE ROW LEVEL SECURITY;
ALTER TABLE goal_entry FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON goal_entry;
CREATE POLICY tenant_isolation ON goal_entry
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON goal_entry TO hubtask_app;

CREATE TABLE IF NOT EXISTS goal_check_in (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  goal_id    uuid NOT NULL,
  author_id  uuid,
  status     text NOT NULL CHECK (status IN ('ON_TRACK', 'AT_RISK', 'OFF_TRACK', 'DONE', 'DROPPED')),
  confidence integer NOT NULL CHECK (confidence BETWEEN 0 AND 10),
  note       text NOT NULL DEFAULT '' CHECK (char_length(note) <= 5000),
  "values"   jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof("values") = 'object'),
  created_at timestamptz NOT NULL,
  CONSTRAINT goal_check_in_goal_fkey
    FOREIGN KEY (tenant_id, goal_id) REFERENCES goal (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS goal_check_in_goal_idx ON goal_check_in (tenant_id, goal_id, created_at DESC);
ALTER TABLE goal_check_in ENABLE ROW LEVEL SECURITY;
ALTER TABLE goal_check_in FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON goal_check_in;
CREATE POLICY tenant_isolation ON goal_check_in
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON goal_check_in TO hubtask_app;

CREATE TABLE IF NOT EXISTS portfolio (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  parent_id   uuid,
  name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
  description text NOT NULL DEFAULT '' CHECK (char_length(description) <= 10000),
  owner_id    uuid,
  status      text NOT NULL CHECK (status IN ('NONE', 'ON_TRACK', 'AT_RISK', 'OFF_TRACK')),
  created_at  timestamptz NOT NULL,
  created_by  uuid,
  updated_at  timestamptz NOT NULL,
  updated_by  uuid,
  version     integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id)
);
ALTER TABLE portfolio ENABLE ROW LEVEL SECURITY;
ALTER TABLE portfolio FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON portfolio;
CREATE POLICY tenant_isolation ON portfolio
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON portfolio TO hubtask_app;

CREATE TABLE IF NOT EXISTS portfolio_item (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  portfolio_id uuid NOT NULL,
  container_id uuid NOT NULL,
  position     integer NOT NULL CHECK (position >= 0),
  start_on     date,
  end_on       date,
  status       text NOT NULL CHECK (status IN ('NONE', 'ON_TRACK', 'AT_RISK', 'OFF_TRACK')),
  PRIMARY KEY (tenant_id, portfolio_id, container_id),
  CHECK (start_on IS NULL OR end_on IS NULL OR end_on >= start_on),
  CONSTRAINT portfolio_item_portfolio_fkey
    FOREIGN KEY (tenant_id, portfolio_id) REFERENCES portfolio (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT portfolio_item_container_fkey
    FOREIGN KEY (tenant_id, container_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
ALTER TABLE portfolio_item ENABLE ROW LEVEL SECURITY;
ALTER TABLE portfolio_item FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON portfolio_item;
CREATE POLICY tenant_isolation ON portfolio_item
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON portfolio_item TO hubtask_app;

CREATE TABLE IF NOT EXISTS portfolio_update (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  portfolio_id uuid NOT NULL,
  author_id    uuid,
  status       text NOT NULL CHECK (status IN ('ON_TRACK', 'AT_RISK', 'OFF_TRACK')),
  summary      text NOT NULL CHECK (char_length(summary) BETWEEN 1 AND 10000),
  created_at   timestamptz NOT NULL,
  CONSTRAINT portfolio_update_portfolio_fkey
    FOREIGN KEY (tenant_id, portfolio_id) REFERENCES portfolio (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS portfolio_update_idx ON portfolio_update (tenant_id, portfolio_id, created_at DESC);
ALTER TABLE portfolio_update ENABLE ROW LEVEL SECURITY;
ALTER TABLE portfolio_update FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON portfolio_update;
CREATE POLICY tenant_isolation ON portfolio_update
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON portfolio_update TO hubtask_app;

CREATE TABLE IF NOT EXISTS dashboard (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id     uuid,
  owner_id   uuid,
  name       text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
  sharing    text NOT NULL CHECK (sharing IN ('PRIVATE', 'SCOPE')),
  widgets    jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(widgets) = 'array'),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  updated_by uuid,
  version    integer NOT NULL DEFAULT 1,
  CONSTRAINT dashboard_hub_fkey
    FOREIGN KEY (tenant_id, hub_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS dashboard_hub_idx ON dashboard (tenant_id, hub_id);
ALTER TABLE dashboard ENABLE ROW LEVEL SECURITY;
ALTER TABLE dashboard FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON dashboard;
CREATE POLICY tenant_isolation ON dashboard
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON dashboard TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
