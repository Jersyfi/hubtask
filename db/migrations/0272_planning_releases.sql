-- Planning (M2, PLN-08): releases and the entries in them. Two new tables, so expand only
-- (versioning-release.md §4).
--
-- * `release` is a version a hub ships: a name, a description, the day it is meant to go out, and
--   once out, when and the page its notes were written to. A page may be trashed and purged on its
--   own, so the page is named without a key; a release keeps naming a page that is gone.
-- * `release_entry` puts an entry into a release; an entry is in at most one.
-- +goose Up
CREATE TABLE IF NOT EXISTS release (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id        uuid NOT NULL,
  name          text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  description   text NOT NULL DEFAULT '' CHECK (char_length(description) <= 2000),
  release_on    date NOT NULL,
  state         text NOT NULL CHECK (state IN ('PLANNED', 'RELEASED')),
  released_at   timestamptz,
  notes_page_id uuid,
  created_at    timestamptz NOT NULL,
  created_by    uuid,
  updated_at    timestamptz NOT NULL,
  version       integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CHECK ((state = 'RELEASED') = (released_at IS NOT NULL)),
  CONSTRAINT release_hub_fkey
    FOREIGN KEY (tenant_id, hub_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS release_hub_idx ON release (tenant_id, hub_id, release_on);
ALTER TABLE release ENABLE ROW LEVEL SECURITY;
ALTER TABLE release FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON release;
CREATE POLICY tenant_isolation ON release
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON release TO hubtask_app;

CREATE TABLE IF NOT EXISTS release_entry (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  release_id uuid NOT NULL,
  entry_id   uuid NOT NULL,
  added_at   timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, release_id, entry_id),
  CONSTRAINT release_entry_release_fkey
    FOREIGN KEY (tenant_id, release_id) REFERENCES release (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT release_entry_entry_fkey
    FOREIGN KEY (tenant_id, entry_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS release_entry_one_idx ON release_entry (tenant_id, entry_id);
ALTER TABLE release_entry ENABLE ROW LEVEL SECURITY;
ALTER TABLE release_entry FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON release_entry;
CREATE POLICY tenant_isolation ON release_entry
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON release_entry TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
