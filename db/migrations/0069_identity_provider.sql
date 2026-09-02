-- The relying-party half of ADR-0005 (H-04): the provider a workspace signs its people in
-- through, and the short-lived flows of authorization code + PKCE.
--
-- What is stored is never a usable credential. The client secret is sealed under E-02's envelope
-- and opened only for a token exchange; the `state` a caller presents back is stored as a hash,
-- the way every other presented token in this schema is.

-- Forward-only and safe for a rolling update: two new tables, nothing altered.

-- +goose Up

-- One provider per workspace, and the primary key says so: a second row cannot exist, so no
-- code path has to decide which of two configurations wins.
CREATE TABLE IF NOT EXISTS identity_provider (
  tenant_id             uuid PRIMARY KEY REFERENCES tenant(id) ON DELETE CASCADE,
  issuer                text NOT NULL CHECK (length(issuer) BETWEEN 1 AND 500),
  client_id             text NOT NULL CHECK (length(client_id) BETWEEN 1 AND 500),
  -- Sealed, never hashed: a token exchange needs the plaintext, so this is the E-02 envelope
  -- and its key label rather than a digest.
  client_secret_enc     bytea NOT NULL,
  client_secret_key_id  text NOT NULL,
  -- The domains inside which a verified address may link an arriving subject to an existing
  -- local account. Empty means no linking at all, which is the safe reading of "not configured":
  -- every subject becomes its own account rather than inheriting somebody else's.
  allowed_email_domains text[] NOT NULL DEFAULT '{}',
  enabled               boolean NOT NULL DEFAULT true,
  created_at            timestamptz NOT NULL,
  updated_at            timestamptz,
  version               integer NOT NULL DEFAULT 1
);

-- One browser round trip, minutes of life. The row carries what the callback must check the ID
-- token against and what the exchange must present, and it is burned on use.
CREATE TABLE IF NOT EXISTS oidc_flow (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  -- Hashed, because the caller holds it and presents it back: `state` is this flow's handle and
  -- is treated as every presented token here is.
  state_hash    bytea NOT NULL,
  -- Kept as they are, deliberately. The verifier has to travel to the provider's token endpoint
  -- and the nonce has to be compared with the ID token's claim, so neither can be a digest. They
  -- buy nothing on their own: an exchange also needs the client secret, which is sealed, so a
  -- reader of this table cannot complete a flow with what is in it.
  code_verifier text NOT NULL,
  nonce         text NOT NULL,
  created_at    timestamptz NOT NULL,
  expires_at    timestamptz NOT NULL,
  -- Consumed rather than deleted, and kept until the sweep: a state presented twice has to meet
  -- a row that refuses it rather than an absence that looks like a typo.
  consumed_at   timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS oidc_flow_state_uq ON oidc_flow (state_hash);
CREATE INDEX IF NOT EXISTS oidc_flow_expiry_idx ON oidc_flow (expires_at);

-- Every tenant-scoped table is behind row level security, and one holding a sealed secret is
-- not where the exception starts.
ALTER TABLE identity_provider ENABLE ROW LEVEL SECURITY;
ALTER TABLE identity_provider FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON identity_provider
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE oidc_flow ENABLE ROW LEVEL SECURITY;
ALTER TABLE oidc_flow FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON oidc_flow
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

-- Explicit rather than left to the default privileges, because those follow the role that
-- creates the table and a migration is not always applied by the same one.
GRANT SELECT, INSERT, UPDATE, DELETE ON identity_provider TO hubtask_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON oidc_flow TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
