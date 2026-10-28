-- The type registry (F-1, UC-FLW-01): a workspace names the kinds of its work - a bug, a feature,
-- a purchase, a configuration item - as rows rather than as values of the enum `item_type`. A type
-- sits on one of the levels (`level`, the values the enum carries) and says what its entries do:
-- whether they are completed, move through a workflow, are records without completion, are
-- scheduled, are estimated, and which types may sit underneath them.
--
-- The three system types - one per level, keyed `task`, `work_package`, `activity` - are declared
-- in the domain and need no row: a workspace that has never shaped them reads them from there, so
-- no path that creates a workspace, restores one or imports one has to seed anything. A system
-- type gains its row the first time a workspace changes it (its name, icon, colour or behaviours),
-- and that row is never archived or deleted.
--
-- Expand only (versioning-release.md §4). `work_item.entry_type` is new and nullable: null is the
-- system type of the entry's level, which is what every entry written before this migration, and
-- every entry an older binary writes during the rollout, means. Its index and the reference's
-- validation follow in 0210, outside a transaction. The enum and the level CHECKs on
-- `work_item` stay: they describe the level, which a type does not change (UC-FLW-01, "Where it
-- ends"), and the enum's casts are read by the previous release's statements, so dropping it is a
-- contract step for a later release.

-- +goose Up
CREATE TABLE IF NOT EXISTS entry_type (
  id               uuid PRIMARY KEY,
  tenant_id        uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  -- The stable key a configuration document, a query and an entry name the type by (F-6).
  key              text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  level            text NOT NULL CHECK (level IN ('TASK', 'WORK_PACKAGE', 'ACTIVITY')),
  system           boolean NOT NULL DEFAULT false,
  name             text NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 80),
  -- An icon's name in the design system's set, never an image.
  icon             text NOT NULL CHECK (icon ~ '^[a-z][a-z0-9-]{0,39}$'),
  -- A theme token, never a colour value (rule 15).
  color_token      text NOT NULL CHECK (color_token ~ '^[a-z][a-z0-9-]{0,39}$'),
  completable      boolean NOT NULL DEFAULT true,
  workflow         boolean NOT NULL DEFAULT false,
  record           boolean NOT NULL DEFAULT false,
  schedulable      boolean NOT NULL DEFAULT true,
  estimable        boolean NOT NULL DEFAULT false,
  -- The keys of the types that may sit directly underneath; empty is every type of the level below.
  allowed_children text[] NOT NULL DEFAULT '{}',
  position         integer NOT NULL DEFAULT 0,
  archived_at      timestamptz,
  created_at       timestamptz NOT NULL,
  updated_at       timestamptz NOT NULL,
  version          integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, key),
  -- A record has no completion (D-07).
  CHECK (NOT (record AND completable)),
  -- A system type is its level's default: it keeps its key, its completion and its place.
  CHECK (NOT system OR (key = lower(level) AND NOT record AND archived_at IS NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS entry_type_system_uq ON entry_type (tenant_id, level) WHERE system;

ALTER TABLE entry_type ENABLE ROW LEVEL SECURITY;
ALTER TABLE entry_type FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON entry_type
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON entry_type TO hubtask_app;

-- The entry's type by key; null is its level's system type, which has no row to point at. The
-- reference keeps a type in use from being deleted (UC-FLW-01 check 5): it can only be archived.
ALTER TABLE work_item ADD COLUMN IF NOT EXISTS entry_type text;
ALTER TABLE work_item
  ADD CONSTRAINT work_item_entry_type_fkey
  FOREIGN KEY (tenant_id, entry_type) REFERENCES entry_type (tenant_id, key) NOT VALID;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
