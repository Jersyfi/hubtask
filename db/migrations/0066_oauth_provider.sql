-- The OAuth2 provider (H-05): the client registry, the grants people consent to, the single-use
-- authorization codes, and the leash that ties an issued session to its grant.
--
-- What is stored is never a credential. A confidential client's secret and every authorization
-- code are stored as hashes under their own purpose labels (D-08's discipline); a public client
-- has no secret at all - PKCE is its whole authentication.

-- Forward-only and safe for a rolling update: three new tables and two nullable columns plus a
-- constraint over columns that are NULL everywhere an old binary wrote them.

-- +goose Up

-- A registered third-party app. Redirect URIs are matched exactly, byte for byte - a prefix or
-- wildcard match is how authorization codes end up on attacker-controlled pages.
CREATE TABLE IF NOT EXISTS oauth_client (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  name          text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
  confidential  boolean NOT NULL,
  secret_hash   bytea,
  redirect_uris text[] NOT NULL,
  created_at    timestamptz NOT NULL,
  created_by    uuid,
  version       integer NOT NULL DEFAULT 1,
  -- A confidential client has a secret and a public one has none: half of either is a client
  -- whose authentication nobody can reason about.
  CHECK (confidential = (secret_hash IS NOT NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS oauth_client_tenant_id_uq ON oauth_client (tenant_id, id);

-- What a person allowed one app: the row they see and revoke beside their sessions. One live
-- grant per person and app - a fresh consent is the newest answer and replaces the scopes.
CREATE TABLE IF NOT EXISTS oauth_grant (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id uuid NOT NULL,
  client_id  uuid NOT NULL,
  scopes     text[] NOT NULL,
  created_at timestamptz NOT NULL,
  revoked_at timestamptz,
  CONSTRAINT oauth_grant_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT oauth_grant_client_fkey FOREIGN KEY (tenant_id, client_id)
    REFERENCES oauth_client (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS oauth_grant_tenant_id_uq ON oauth_grant (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS oauth_grant_live_uq ON oauth_grant (account_id, client_id)
  WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS oauth_grant_account_idx ON oauth_grant (account_id, created_at DESC);

-- A single-use authorization code: minutes of life, consumed by its exchange, and kept briefly
-- afterwards because a consumed hash presented again is the replay the refusal needs to see.
CREATE TABLE IF NOT EXISTS oauth_code (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  client_id      uuid NOT NULL,
  account_id     uuid NOT NULL,
  grant_id       uuid NOT NULL,
  code_hash      bytea NOT NULL,
  code_challenge text NOT NULL,
  redirect_uri   text NOT NULL,
  created_at     timestamptz NOT NULL,
  expires_at     timestamptz NOT NULL,
  consumed_at    timestamptz,
  CONSTRAINT oauth_code_grant_fkey FOREIGN KEY (tenant_id, grant_id)
    REFERENCES oauth_grant (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS oauth_code_hash_uq ON oauth_code (code_hash);

-- The leash (security.md §5, "the grant as their leash"): a session issued through an exchange
-- names its grant and carries the grant's scopes; revoking the grant ends its sessions the way
-- refresh reuse would. NULL scopes is a person's own session, bounded by nothing but their role.
ALTER TABLE session
  ADD COLUMN IF NOT EXISTS grant_id uuid,
  ADD COLUMN IF NOT EXISTS scopes text[];
ALTER TABLE session
  ADD CONSTRAINT session_grant_fkey FOREIGN KEY (tenant_id, grant_id)
    REFERENCES oauth_grant (tenant_id, id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS session_grant_idx ON session (grant_id) WHERE grant_id IS NOT NULL;

-- Every tenant-scoped table is behind row level security, and credential tables least of all
-- are an exception.
ALTER TABLE oauth_client ENABLE ROW LEVEL SECURITY;
ALTER TABLE oauth_client FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON oauth_client
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE oauth_grant ENABLE ROW LEVEL SECURITY;
ALTER TABLE oauth_grant FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON oauth_grant
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE oauth_code ENABLE ROW LEVEL SECURITY;
ALTER TABLE oauth_code FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON oauth_code
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

-- The grant is explicit rather than left to the default privileges, because those follow the
-- role that creates the table and a migration is not always applied by the same one.
GRANT SELECT, INSERT, UPDATE, DELETE ON oauth_client TO hubtask_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON oauth_grant TO hubtask_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON oauth_code TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
