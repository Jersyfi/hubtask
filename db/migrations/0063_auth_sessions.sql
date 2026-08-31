-- The sign-in the system has never had (H-01, security.md §5): a session somebody can see and
-- revoke, the rotating refresh family that hangs off it, the sign-in attempt ledger behind T-02's
-- lockout, and the redemption token that closes the invitation loop.
--
-- What is stored is never a credential. The access token is not stored at all - it verifies by
-- its signature under the installation secret's purpose label. The refresh token and the
-- redemption token are stored as hashes under their own purpose labels, so a hash from one table
-- can never be presented as the other, or as a PAT, a cursor or a feed token (D-08's discipline).

-- Forward-only and safe for a rolling update: three new tables and two nullable columns, none of
-- which an old binary reads or writes.

-- +goose Up

-- A sign-in. The row is what /auth/sessions lists and what revocation stamps; both tokens of the
-- pair point at it, so ending it ends them together.
CREATE TABLE IF NOT EXISTS session (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id   uuid NOT NULL,
  created_at   timestamptz NOT NULL,
  -- The retention anchor (data-retention.md §3, SESSION): when the session last acted, written
  -- back at most once per interval so the read path does not become a write path.
  last_seen_at timestamptz,
  -- The client-binding hint T-01 asks to log: the user agent as the client introduced itself,
  -- and the network coarsened at recording time - an IPv4 /24 or an IPv6 /48, never the full
  -- address. Both are personal data and carry catalogue rows; neither reaches a log or a trace.
  user_agent   text,
  ip_class     text,
  -- When the session ends of its own accord: the horizon of its newest refresh token. Rotation
  -- slides it; a session nobody refreshes runs out with the token that could have renewed it.
  expires_at   timestamptz NOT NULL,
  revoked_at   timestamptz,
  CONSTRAINT session_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS session_tenant_id_uq ON session (tenant_id, id);
-- The listing: one's own sessions, newest first.
CREATE INDEX IF NOT EXISTS session_account_idx ON session (account_id, created_at DESC);

-- One refresh token of a session's family. Rotation retires a row and inserts the next; the
-- retired rows stay until the session goes, because a retired hash presented again is the reuse
-- signal T-01 exists for - deleting it would delete the evidence that distinguishes theft from
-- an unknown token.
CREATE TABLE IF NOT EXISTS session_refresh_token (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  session_id  uuid NOT NULL,
  token_hash  bytea NOT NULL,
  created_at  timestamptz NOT NULL,
  expires_at  timestamptz NOT NULL,
  -- Set when the token was exchanged. A rotated token presented again means two holders, and
  -- the whole family dies with the session.
  rotated_at  timestamptz,
  CONSTRAINT session_refresh_token_session_fkey FOREIGN KEY (tenant_id, session_id)
    REFERENCES session (tenant_id, id) ON DELETE CASCADE
);
-- The lookup the public refresh route makes, and the reason the token names its own tenant: the
-- hash covers the whole presented string, tenant half included, and is unique across the
-- installation - a token rewritten to quote another tenant matches nothing at all.
CREATE UNIQUE INDEX IF NOT EXISTS session_refresh_token_hash_uq
  ON session_refresh_token (token_hash);
CREATE INDEX IF NOT EXISTS session_refresh_token_session_idx
  ON session_refresh_token (session_id);

-- The sign-in attempt ledger (T-02): failures per account and per source network, with the
-- progressive delay and the lockout computed from it. The subject is stored only as a hash under
-- its own purpose label - the ledger must count attempts against addresses that hold no account
-- without becoming a list of guessed addresses.
CREATE TABLE IF NOT EXISTS auth_attempt (
  tenant_id       uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  subject_hash    bytea NOT NULL,
  failures        integer NOT NULL DEFAULT 0,
  last_failure_at timestamptz,
  locked_until    timestamptz,
  PRIMARY KEY (tenant_id, subject_hash)
);

-- The redemption token the invitation mints (data-catalog.md §7.5): hashed under its own purpose
-- label, shown once, and dead on redemption. On the account rather than in a table of its own,
-- because there is exactly one open invitation per invited account and the token *is* that
-- invitation's - a second live token would be a second door into the same account.
ALTER TABLE account
  ADD COLUMN IF NOT EXISTS redemption_token_hash bytea,
  ADD COLUMN IF NOT EXISTS redemption_expires_at timestamptz;

-- The lookup the public redemption route makes, with the refresh hash's reasoning.
CREATE UNIQUE INDEX IF NOT EXISTS account_redemption_token_uq
  ON account (redemption_token_hash)
  WHERE redemption_token_hash IS NOT NULL;

-- Every tenant-scoped table is behind row level security, and credential tables least of all are
-- an exception.
ALTER TABLE session ENABLE ROW LEVEL SECURITY;
ALTER TABLE session FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON session
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE session_refresh_token ENABLE ROW LEVEL SECURITY;
ALTER TABLE session_refresh_token FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON session_refresh_token
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE auth_attempt ENABLE ROW LEVEL SECURITY;
ALTER TABLE auth_attempt FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON auth_attempt
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

-- The grant is explicit rather than left to the default privileges, because those follow the role
-- that creates the table and a migration is not always applied by the same one.
GRANT SELECT, INSERT, UPDATE, DELETE ON session TO hubtask_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON session_refresh_token TO hubtask_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON auth_attempt TO hubtask_app;

-- Sign-in needs a tenant before it can check a password (0.6.0 decision 3): accounts are
-- per-tenant, so the workspace has to be known before `SET LOCAL app.tenant_id` can bound the
-- credential lookup - and the `tenant` table's own policy shows a transaction nothing until then.
--
-- SECURITY DEFINER for the reason `subject_tenants` is (0044): reading `tenant` without a context
-- is the owner's right and the application role does not hold it. Narrow by construction: it
-- answers exactly one identifier or none, never a listing - a slug names its tenant, and NULL
-- answers the single-mode installation's only row and refuses to choose among several. What is
-- done with the identifier stays behind row level security exactly as before: naming a tenant
-- gains nothing but the context in which every policy still applies.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION resolve_tenant(tenant_slug text) RETURNS uuid
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT id FROM tenant
  WHERE deleted_at IS NULL
    AND (
      (tenant_slug IS NOT NULL AND slug = lower(tenant_slug))
      OR (tenant_slug IS NULL
          AND (SELECT count(*) FROM tenant WHERE deleted_at IS NULL) = 1)
    )
  LIMIT 1
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION resolve_tenant(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION resolve_tenant(text) TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
