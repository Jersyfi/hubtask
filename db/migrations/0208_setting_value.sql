-- One storage shape for the settings a workspace and the places in it decide (the settings
-- catalogue, domain-model.md §2.4): a dotted key, its value as JSON, how the level holds it, and
-- what it leaves the levels below. The installation's and a plan's levels keep their own tables
-- (instance_setting, plan_setting) in the same shape - a key and a JSON value with a lock - because
-- they are nobody's rows and carry no row level security.
--
-- `level` and `scope_id` say where: the workspace (scope_id is the tenant), a hub or a collection
-- (scope_id is the container), a person (scope_id is the account). No foreign key on scope_id: it
-- points into three tables, and a place that goes takes its rows with it through the use case that
-- removes it; a row left behind decides nothing for a place that no longer exists.

-- +goose Up
CREATE TABLE IF NOT EXISTS setting_value (
  -- An identity of its own, so that a backup carries a row like every other (tenant-export.md §6).
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  level      text NOT NULL CHECK (level IN ('WORKSPACE', 'HUB', 'COLLECTION', 'PERSON')),
  scope_id   uuid NOT NULL,
  key        text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$'),
  -- Null where the level locks or restricts without restating the value above.
  value      jsonb,
  hold       text NOT NULL DEFAULT 'SET' CHECK (hold IN ('SET', 'LOCK', 'RESTRICT')),
  -- A restriction's range or subset: {"minimum": n, "maximum": n, "options": [...]}.
  allowed    jsonb,
  set_by     uuid,
  updated_at timestamptz NOT NULL,
  UNIQUE (tenant_id, level, scope_id, key),
  CHECK (value IS NOT NULL OR hold <> 'SET'),
  CHECK ((allowed IS NOT NULL) = (hold = 'RESTRICT'))
);

ALTER TABLE setting_value ENABLE ROW LEVEL SECURITY;
ALTER TABLE setting_value FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON setting_value
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON setting_value TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
