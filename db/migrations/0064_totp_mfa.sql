-- The second factor a tenant can demand (H-02, security.md §5): the TOTP enrolment with its
-- sealed secret, the ten single-use recovery codes, and the pending credential a two-step
-- sign-in hands out between the password and the code.
--
-- What is stored is never usable as presented. The TOTP secret is sealed through E-02's envelope
-- encryption under its own purpose - the server must read it back to verify codes, which is what
-- separates it from every hashed credential. Recovery codes and pending tokens are stored as
-- hashes under their own purpose labels, D-08's discipline.

-- Forward-only and safe for a rolling update: three new tables, nothing an old binary touches.

-- +goose Up

-- One enrolment per account. `confirmed_at` is what arms it: until a valid code confirms, the
-- row changes nothing about sign-in, because an unconfirmed enrolment protects nobody and locks
-- nobody out. `last_step` is the replay refusal - the highest accepted RFC 6238 time step, so
-- the same code never verifies twice.
CREATE TABLE IF NOT EXISTS account_mfa (
  account_id    uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  secret_enc    bytea NOT NULL,
  secret_key_id text NOT NULL,
  confirmed_at  timestamptz,
  last_step     bigint,
  created_at    timestamptz NOT NULL,
  updated_at    timestamptz NOT NULL,
  CONSTRAINT account_mfa_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);

-- Ten per enrolment, replaced wholesale on re-enrolment, each burned by its first use. The hash
-- is unique across the installation for the refresh token's reason: a code presented under the
-- wrong account or tenant matches nothing at all.
CREATE TABLE IF NOT EXISTS account_recovery_code (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id uuid NOT NULL,
  code_hash  bytea NOT NULL,
  used_at    timestamptz,
  created_at timestamptz NOT NULL,
  CONSTRAINT account_recovery_code_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS account_recovery_code_hash_uq
  ON account_recovery_code (code_hash);
CREATE INDEX IF NOT EXISTS account_recovery_code_account_idx
  ON account_recovery_code (account_id);

-- The pending credential of a two-step sign-in: a row with the session machinery's discipline -
-- short-lived, single-use, hashed under its own purpose label - and deliberately not a session.
-- Its purpose says what it may complete: TOTP presents a code, ENROLL is the enforcement route
-- for an administrator who is not yet enrolled. The client hint rides along so the session that
-- eventually opens records the sign-in's own device, not the confirmation call's.
CREATE TABLE IF NOT EXISTS auth_pending (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id  uuid NOT NULL,
  token_hash  bytea NOT NULL,
  purpose     text NOT NULL CHECK (purpose IN ('TOTP', 'ENROLL')),
  user_agent  text,
  ip_class    text,
  created_at  timestamptz NOT NULL,
  expires_at  timestamptz NOT NULL,
  consumed_at timestamptz,
  CONSTRAINT auth_pending_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS auth_pending_token_uq ON auth_pending (token_hash);

-- Every tenant-scoped table is behind row level security, and credential tables least of all
-- are an exception.
ALTER TABLE account_mfa ENABLE ROW LEVEL SECURITY;
ALTER TABLE account_mfa FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON account_mfa
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE account_recovery_code ENABLE ROW LEVEL SECURITY;
ALTER TABLE account_recovery_code FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON account_recovery_code
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE auth_pending ENABLE ROW LEVEL SECURITY;
ALTER TABLE auth_pending FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON auth_pending
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

-- The grant is explicit rather than left to the default privileges, because those follow the
-- role that creates the table and a migration is not always applied by the same one.
GRANT SELECT, INSERT, UPDATE, DELETE ON account_mfa TO hubtask_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON account_recovery_code TO hubtask_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON auth_pending TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
