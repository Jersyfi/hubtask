-- The configuration document's versions, agents' plans waiting for a person, and templates (F-6,
-- D-15 … D-17). Expand only (versioning-release.md §4): four new tables, nothing an older build
-- reads changes.
--
-- * `config_revision` is one configuration version per apply: the document in force after it,
--   its fingerprint, who applied it and who proposed it, and the counts - configuration and
--   counts only, never content (rule 10).
-- * `config_proposal` is an agent's plan kept for a person (D-16): the document it asks for, the
--   fingerprint of the configuration it was planned on, and how it was decided.
-- * `config_template` is one version of a workspace's template; a version is never rewritten.
-- * `config_template_use` is the template a workspace took and the template's document then - the
--   base of a three-way upgrade (D-17).
--
-- The actors are bare identifiers rather than foreign keys, as in the trail: a version outlives
-- the account that applied it.

-- +goose Up
CREATE TABLE IF NOT EXISTS config_revision (
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  version        integer NOT NULL CHECK (version > 0),
  document       jsonb NOT NULL,
  hash           text NOT NULL,
  plan_hash      text NOT NULL,
  actor_kind     text NOT NULL,
  actor_id       uuid,
  proposed_by    uuid,
  rolled_back_to integer,
  template       text,
  added          integer NOT NULL DEFAULT 0,
  changed        integer NOT NULL DEFAULT 0,
  removed        integer NOT NULL DEFAULT 0,
  created_at     timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, version)
);

ALTER TABLE config_revision ENABLE ROW LEVEL SECURITY;
ALTER TABLE config_revision FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON config_revision
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
-- A version is evidence: written once, never changed.
GRANT SELECT, INSERT ON config_revision TO hubtask_app;

CREATE TABLE IF NOT EXISTS config_proposal (
  id                  uuid PRIMARY KEY,
  tenant_id           uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  state               text NOT NULL CHECK (state IN ('WAITING', 'APPLIED', 'DECLINED')),
  document            jsonb NOT NULL,
  mode                text NOT NULL CHECK (mode IN ('MERGE', 'REPLACE')),
  base_hash           text NOT NULL,
  plan_hash           text NOT NULL,
  template            text,
  added               integer NOT NULL DEFAULT 0,
  changed             integer NOT NULL DEFAULT 0,
  removed             integer NOT NULL DEFAULT 0,
  hides_data          boolean NOT NULL DEFAULT false,
  changes_permissions boolean NOT NULL DEFAULT false,
  proposed_by         uuid,
  decided_by          uuid,
  created_at          timestamptz NOT NULL,
  decided_at          timestamptz,
  UNIQUE (tenant_id, id)
);
CREATE INDEX IF NOT EXISTS config_proposal_waiting_idx
  ON config_proposal (tenant_id, created_at DESC) WHERE state = 'WAITING';

ALTER TABLE config_proposal ENABLE ROW LEVEL SECURITY;
ALTER TABLE config_proposal FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON config_proposal
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE ON config_proposal TO hubtask_app;

CREATE TABLE IF NOT EXISTS config_template (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key        text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,62}$'),
  version    integer NOT NULL CHECK (version > 0),
  name       text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  origin     text NOT NULL CHECK (origin IN ('WORKSPACE', 'OWN')),
  document   jsonb NOT NULL,
  created_by uuid,
  created_at timestamptz NOT NULL,
  UNIQUE (tenant_id, id),
  CONSTRAINT config_template_version_uq UNIQUE (tenant_id, key, version)
);

ALTER TABLE config_template ENABLE ROW LEVEL SECURITY;
ALTER TABLE config_template FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON config_template
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT ON config_template TO hubtask_app;

CREATE TABLE IF NOT EXISTS config_template_use (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  template_key text NOT NULL,
  version      integer NOT NULL,
  document     jsonb NOT NULL,
  applied_at   timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, template_key)
);

ALTER TABLE config_template_use ENABLE ROW LEVEL SECURITY;
ALTER TABLE config_template_use FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON config_template_use
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE ON config_template_use TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
