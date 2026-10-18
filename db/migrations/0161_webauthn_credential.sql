-- Passkeys and security keys: the WebAuthn credentials of an account (identity.md §23).
--
-- An account holds up to ten, beside at most one authenticator app (`account_mfa`), so they live in a
-- table of their own rather than a column of the factor's. What is kept is what a later ceremony is
-- checked against: the credential's identifier, its public key as the COSE bytes the authenticator
-- sent, the signature counter, the backup flags, the transport hints - and the person's name for it,
-- which is user content (rule 10), with when it was added and last used. The user handle is the
-- opaque value the authenticator stores for the account, never the address. Under the tenant's row
-- security like every credential table (rule 3); the account's removal takes them with it.
--
-- A registration's challenge waits in `auth_pending` under the purpose PASSKEY_REGISTER: bound to the
-- session that asked, after a step-up, with the user handle the new credential will carry. Its two
-- columns are set exactly for that purpose.
--
-- Expand only: a new table nothing older reads, two nullable columns the previous release never
-- writes, and a purpose it never presents - so its rows pass both new checks (rule 12).

-- +goose Up
CREATE TABLE IF NOT EXISTS webauthn_credential (
  id              uuid PRIMARY KEY,
  tenant_id       uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id      uuid NOT NULL,
  credential_id   bytea NOT NULL CHECK (octet_length(credential_id) BETWEEN 1 AND 1023),
  public_key      bytea NOT NULL CHECK (octet_length(public_key) BETWEEN 1 AND 2048),
  algorithm       integer NOT NULL CHECK (algorithm IN (-7, -8, -257)),
  sign_count      bigint NOT NULL DEFAULT 0 CHECK (sign_count BETWEEN 0 AND 4294967295),
  user_handle     bytea NOT NULL CHECK (octet_length(user_handle) = 32),
  backup_eligible boolean NOT NULL,
  backed_up       boolean NOT NULL,
  transports      text[] NOT NULL DEFAULT '{}' CHECK (cardinality(transports) <= 6),
  name            text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 64),
  created_at      timestamptz NOT NULL,
  last_used_at    timestamptz,
  CONSTRAINT webauthn_credential_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT webauthn_credential_backup_state CHECK (backup_eligible OR NOT backed_up)
);

-- One credential once per workspace: the identifier a sign-in looks it up by (identity.md §23.3).
-- Built without CONCURRENTLY: the table was created in this same migration and nobody reads it yet.
CREATE UNIQUE INDEX IF NOT EXISTS webauthn_credential_credential_uq
  ON webauthn_credential (tenant_id, credential_id);
CREATE INDEX IF NOT EXISTS webauthn_credential_account_idx
  ON webauthn_credential (tenant_id, account_id, created_at);

ALTER TABLE webauthn_credential ENABLE ROW LEVEL SECURITY;
ALTER TABLE webauthn_credential FORCE ROW LEVEL SECURITY;
-- +goose StatementBegin
DO $policy$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'webauthn_credential' AND policyname = 'tenant_isolation') THEN
    CREATE POLICY tenant_isolation ON webauthn_credential
      USING (tenant_id = current_tenant_id())
      WITH CHECK (tenant_id = current_tenant_id());
  END IF;
END $policy$;
-- +goose StatementEnd

GRANT SELECT, INSERT, UPDATE, DELETE ON webauthn_credential TO hubtask_app;

-- The registration's challenge.
ALTER TABLE auth_pending ADD COLUMN IF NOT EXISTS session_id uuid;
ALTER TABLE auth_pending ADD COLUMN IF NOT EXISTS user_handle bytea;

ALTER TABLE auth_pending DROP CONSTRAINT IF EXISTS auth_pending_session_fkey;
ALTER TABLE auth_pending ADD CONSTRAINT auth_pending_session_fkey FOREIGN KEY (tenant_id, session_id)
  REFERENCES session (tenant_id, id) ON DELETE CASCADE;

ALTER TABLE auth_pending DROP CONSTRAINT IF EXISTS auth_pending_purpose_check;
ALTER TABLE auth_pending ADD CONSTRAINT auth_pending_purpose_check
  CHECK (purpose IN ('TOTP', 'ENROLL', 'RESET', 'PASSWORD', 'LINK', 'CONNECT', 'ADDRESS', 'TERMS',
                     'PASSKEY_REGISTER'));

-- A registration's session and handle, and nothing of either on any other row.
ALTER TABLE auth_pending DROP CONSTRAINT IF EXISTS auth_pending_passkey_whole;
ALTER TABLE auth_pending ADD CONSTRAINT auth_pending_passkey_whole
  CHECK (CASE WHEN purpose = 'PASSKEY_REGISTER'
              THEN session_id IS NOT NULL AND user_handle IS NOT NULL AND octet_length(user_handle) = 32
              ELSE session_id IS NULL AND user_handle IS NULL END);

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
