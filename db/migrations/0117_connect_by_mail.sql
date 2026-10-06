-- A workspace without the password connects its provider by mail (ADR-0078 §1, SC-33).
--
-- Where a workspace switched the password off, *Forgot your password?* mails an account that no
-- provider switched on there lets in a link to connect one instead of a link to set a password. The
-- link is a pending credential of its own purpose, CONNECT: thirty minutes, single use, the reset
-- link's discipline. It is checked when the provider flow starts - without being spent - and the
-- flow keeps which credential it carries, `oidc_flow.invited_account_id`'s precedent (migration
-- 0116): the token itself never travels to the provider and is not stored a second time. Only an
-- arrival that connects the provider spends it, in the same transaction.
--
-- `link_proof` says what proved the account when a provider identity was connected: the password at
-- the LINK step, or the mailbox together with a fresh sign-in at the provider. It travels with the
-- link into the second factor's step, where the connection is written, so that the trail entry says
-- which proof it was. NULL is a row of the previous binary, which knew only the password.
--
-- The flow's key is the tenant-scoped one, so a flow can carry only a credential of its own
-- workspace; it dies with the credential (a swept, expired link leaves no flow that could name it).
-- Every column is nullable, so the previous binary keeps writing the rows it knows.
-- +goose Up
ALTER TABLE auth_pending DROP CONSTRAINT IF EXISTS auth_pending_purpose_check;
ALTER TABLE auth_pending ADD CONSTRAINT auth_pending_purpose_check
  CHECK (purpose IN ('TOTP', 'ENROLL', 'RESET', 'PASSWORD', 'LINK', 'CONNECT'));

ALTER TABLE auth_pending
  ADD COLUMN IF NOT EXISTS link_proof text CHECK (link_proof IN ('PASSWORD', 'MAILBOX'));

-- A row lives minutes, so the table is small and the index is built in a moment.
CREATE UNIQUE INDEX IF NOT EXISTS auth_pending_tenant_id_uq ON auth_pending (tenant_id, id);

ALTER TABLE oidc_flow ADD COLUMN IF NOT EXISTS pending_id uuid;
ALTER TABLE oidc_flow DROP CONSTRAINT IF EXISTS oidc_flow_pending_fkey;
ALTER TABLE oidc_flow ADD CONSTRAINT oidc_flow_pending_fkey
  FOREIGN KEY (tenant_id, pending_id) REFERENCES auth_pending (tenant_id, id) ON DELETE CASCADE;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
