-- The assets and configuration module (M6): a workspace's CI classes, relation types and sources,
-- and a hub's configuration items, their relations and the reconciliation's records.
--
-- * `cmdb_class`, `cmdb_relation_type` and `cmdb_source` are the workspace's own configuration
--   over what Hubtask ships (the shipped hierarchy, catalogue and the `manual` source are
--   declared in the domain and have no row).
-- * `cmdb_ci` sits in a hub. `attributes` holds its class's typed values, `provenance` per
--   attribute the source, run and moment that last wrote it; `protection` is the need a person
--   set ({c, i, a, fixed, reason}); the inherited need is computed, never stored.
-- * `cmdb_ci_identifier` is the identifier index the reconciliation looks up: the name, every
--   identifier attribute and each source's own key, normalised, kept by the application in the
--   writing transaction.
-- * `cmdb_relation` joins two CIs of one hub; it is unique per type and direction and remembers
--   its source and when it was first and last seen.
-- * `cmdb_run`, `cmdb_hold` and `cmdb_duplicate` are the reconciliation's explanation, the items
--   it held for review and the duplicates it opened to merge - tenant records, never log lines
--   (rule 10).
--
-- Expand only: new tables nothing older reads (rule 12). No CI is ever deleted by a use case: a
-- merged duplicate is retired; a hub's or workspace's deletion takes its CIs with it.
-- +goose Up
CREATE TABLE IF NOT EXISTS cmdb_class (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key            text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  parent_key     text NOT NULL,
  name           text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  icon           text NOT NULL DEFAULT '',
  abstract       boolean NOT NULL DEFAULT false,
  attributes     jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(attributes) = 'array'),
  rules          jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(rules) = 'array'),
  required       jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(required) = 'array'),
  staleness_days integer NOT NULL DEFAULT 0 CHECK (staleness_days BETWEEN 0 AND 3650),
  archived_at    timestamptz,
  created_at     timestamptz NOT NULL,
  updated_at     timestamptz NOT NULL,
  version        integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, key)
);

CREATE TABLE IF NOT EXISTS cmdb_relation_type (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key          text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  name         text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  reverse_name text NOT NULL CHECK (char_length(reverse_name) BETWEEN 1 AND 120),
  impact       text NOT NULL CHECK (impact IN ('NONE', 'SOURCE', 'TARGET', 'BOTH')),
  pairs        jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(pairs) = 'array'),
  archived_at  timestamptz,
  created_at   timestamptz NOT NULL,
  updated_at   timestamptz NOT NULL,
  version      integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, key)
);

CREATE TABLE IF NOT EXISTS cmdb_source (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key         text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$' AND key <> 'manual'),
  kind        text NOT NULL CHECK (kind IN ('FILE', 'API', 'CONNECTOR')),
  rank        integer NOT NULL CHECK (rank BETWEEN 1 AND 1000),
  classes     text[] NOT NULL DEFAULT '{}',
  may_create  boolean NOT NULL DEFAULT false,
  mapping     jsonb,
  last_run_at timestamptz,
  created_at  timestamptz NOT NULL,
  updated_at  timestamptz NOT NULL,
  version     integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, key)
);

CREATE TABLE IF NOT EXISTS cmdb_ci (
  id               uuid PRIMARY KEY,
  tenant_id        uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id           uuid NOT NULL,
  class_key        text NOT NULL,
  name             text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
  stage            text NOT NULL CHECK (stage IN
                     ('PLANNED', 'ORDERED', 'IN_STOCK', 'IN_USE', 'MAINTENANCE', 'RETIRED', 'DISPOSED')),
  status           text NOT NULL CHECK (status IN ('OK', 'PENDING', 'BLOCKED', 'MERGED')),
  attributes       jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(attributes) = 'object'),
  provenance       jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(provenance) = 'object'),
  owner_account_id uuid,
  owner_group_id   uuid,
  location_ci_id   uuid,
  protection       jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(protection) = 'object'),
  review_due       date,
  quantity         integer NOT NULL DEFAULT 1 CHECK (quantity BETWEEN 1 AND 1000000),
  last_seen_at     timestamptz,
  merged_into      uuid,
  created_by       uuid NOT NULL,
  created_at       timestamptz NOT NULL,
  updated_by       uuid NOT NULL,
  updated_at       timestamptz NOT NULL,
  version          integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT cmdb_ci_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS cmdb_ci_hub_idx ON cmdb_ci (tenant_id, hub_id, class_key, id);

CREATE TABLE IF NOT EXISTS cmdb_ci_identifier (
  tenant_id uuid NOT NULL,
  ci_id     uuid NOT NULL,
  hub_id    uuid NOT NULL,
  name      text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  value     text NOT NULL CHECK (char_length(value) BETWEEN 1 AND 500),
  PRIMARY KEY (tenant_id, ci_id, name),
  CONSTRAINT cmdb_ci_identifier_ci_fkey FOREIGN KEY (tenant_id, ci_id)
    REFERENCES cmdb_ci (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS cmdb_ci_identifier_lookup_idx ON cmdb_ci_identifier (tenant_id, hub_id, name, value);

CREATE TABLE IF NOT EXISTS cmdb_relation (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL,
  hub_id       uuid NOT NULL,
  type_key     text NOT NULL,
  source_ci_id uuid NOT NULL,
  target_ci_id uuid NOT NULL CHECK (target_ci_id <> source_ci_id),
  data_source  text NOT NULL,
  first_seen   timestamptz NOT NULL,
  last_seen    timestamptz NOT NULL,
  created_by   uuid NOT NULL,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, type_key, source_ci_id, target_ci_id),
  CONSTRAINT cmdb_relation_source_fkey FOREIGN KEY (tenant_id, source_ci_id)
    REFERENCES cmdb_ci (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT cmdb_relation_target_fkey FOREIGN KEY (tenant_id, target_ci_id)
    REFERENCES cmdb_ci (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS cmdb_relation_source_idx ON cmdb_relation (tenant_id, source_ci_id);
CREATE INDEX IF NOT EXISTS cmdb_relation_target_idx ON cmdb_relation (tenant_id, target_ci_id);
CREATE INDEX IF NOT EXISTS cmdb_relation_hub_idx ON cmdb_relation (tenant_id, hub_id);

CREATE TABLE IF NOT EXISTS cmdb_run (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL,
  hub_id     uuid NOT NULL,
  source_key text NOT NULL,
  actor_id   uuid NOT NULL,
  started_at timestamptz NOT NULL,
  counts     jsonb NOT NULL DEFAULT '{}'::jsonb,
  decisions  jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(decisions) = 'array'),
  UNIQUE (tenant_id, id),
  CONSTRAINT cmdb_run_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS cmdb_run_hub_idx ON cmdb_run (tenant_id, hub_id, started_at DESC);

CREATE TABLE IF NOT EXISTS cmdb_hold (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL,
  hub_id      uuid NOT NULL,
  run_id      uuid NOT NULL,
  source_key  text NOT NULL,
  item        jsonb NOT NULL,
  reason      text NOT NULL,
  created_at  timestamptz NOT NULL,
  resolved_at timestamptz,
  resolved_by uuid,
  UNIQUE (tenant_id, id),
  CONSTRAINT cmdb_hold_run_fkey FOREIGN KEY (tenant_id, run_id)
    REFERENCES cmdb_run (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS cmdb_hold_hub_idx ON cmdb_hold (tenant_id, hub_id, created_at DESC) WHERE resolved_at IS NULL;

CREATE TABLE IF NOT EXISTS cmdb_duplicate (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL,
  hub_id      uuid NOT NULL,
  ci_ids      uuid[] NOT NULL CHECK (cardinality(ci_ids) BETWEEN 2 AND 50),
  rule        text NOT NULL,
  created_at  timestamptz NOT NULL,
  resolved_at timestamptz,
  resolved_by uuid,
  survivor_id uuid,
  UNIQUE (tenant_id, id),
  CONSTRAINT cmdb_duplicate_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS cmdb_duplicate_hub_idx ON cmdb_duplicate (tenant_id, hub_id, created_at DESC) WHERE resolved_at IS NULL;

ALTER TABLE cmdb_class ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_class FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_class;
CREATE POLICY tenant_isolation ON cmdb_class
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_relation_type ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_relation_type FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_relation_type;
CREATE POLICY tenant_isolation ON cmdb_relation_type
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_source ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_source FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_source;
CREATE POLICY tenant_isolation ON cmdb_source
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_ci ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_ci FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_ci;
CREATE POLICY tenant_isolation ON cmdb_ci
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_ci_identifier ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_ci_identifier FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_ci_identifier;
CREATE POLICY tenant_isolation ON cmdb_ci_identifier
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_relation ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_relation FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_relation;
CREATE POLICY tenant_isolation ON cmdb_relation
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_run ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_run FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_run;
CREATE POLICY tenant_isolation ON cmdb_run
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_hold ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_hold FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_hold;
CREATE POLICY tenant_isolation ON cmdb_hold
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_duplicate ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_duplicate FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_duplicate;
CREATE POLICY tenant_isolation ON cmdb_duplicate
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

GRANT SELECT, INSERT, UPDATE ON cmdb_class, cmdb_relation_type, cmdb_source, cmdb_ci TO hubtask_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON cmdb_ci_identifier, cmdb_relation TO hubtask_app;
GRANT SELECT, INSERT, UPDATE ON cmdb_run, cmdb_hold, cmdb_duplicate TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
