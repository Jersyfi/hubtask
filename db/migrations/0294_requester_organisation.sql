-- A portal's organisations (accepted.md D, UC-SVC-11): a group of requesters by mail domain. Who
-- belongs is read from each account's address when asked, so nothing is stored per person and
-- nothing goes stale. A mail domain belongs to at most one organisation of a workspace: a trigger
-- refuses an overlap under a lock per workspace, since a unique index cannot reach into an array.
-- A new table, so expand only (versioning-release.md §4).
-- +goose Up
CREATE TABLE IF NOT EXISTS requester_organisation (
  id               uuid PRIMARY KEY,
  tenant_id        uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key              text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  name             text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  domains          text[] NOT NULL CHECK (cardinality(domains) BETWEEN 1 AND 20),
  shares_requests  boolean NOT NULL DEFAULT false,
  created_at       timestamptz NOT NULL,
  updated_at       timestamptz NOT NULL,
  version          integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT requester_organisation_key_uq UNIQUE (tenant_id, key)
);
CREATE INDEX IF NOT EXISTS requester_organisation_domains_idx
  ON requester_organisation USING gin (domains);
ALTER TABLE requester_organisation ENABLE ROW LEVEL SECURITY;
ALTER TABLE requester_organisation FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON requester_organisation;
CREATE POLICY tenant_isolation ON requester_organisation
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON requester_organisation TO hubtask_app;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION requester_organisation_domains_unique() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM pg_advisory_xact_lock(hashtextextended('requester_organisation:' || NEW.tenant_id::text, 0));
  IF EXISTS (
    SELECT 1 FROM requester_organisation o
    WHERE o.tenant_id = NEW.tenant_id AND o.id <> NEW.id AND o.domains && NEW.domains
  ) THEN
    RAISE EXCEPTION 'a mail domain belongs to another organisation'
      USING ERRCODE = 'unique_violation', CONSTRAINT = 'requester_organisation_domain_uq';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER IF EXISTS requester_organisation_domains_unique ON requester_organisation;
CREATE TRIGGER requester_organisation_domains_unique
  BEFORE INSERT OR UPDATE OF domains ON requester_organisation
  FOR EACH ROW EXECUTE FUNCTION requester_organisation_domains_unique();

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
