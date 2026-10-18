-- A passkey sign-in's challenge (identity.md §23.3, §23.4).
--
-- Signing in with a passkey names nobody: the device offers the passkeys it holds for this
-- workspace, so the challenge the server issues belongs to no account yet. It waits in auth_pending
-- under its own purpose, PASSKEY_SIGN_IN - hashed under its own prefix like every pending token,
-- five minutes, single use - and the two checks that admitted an account-less row only for a TERMS
-- arrival admit it too.
--
-- Expand only: the previous release never writes the purpose, and every row it writes still passes
-- both widened checks (rule 12).

-- +goose Up
ALTER TABLE auth_pending DROP CONSTRAINT IF EXISTS auth_pending_purpose_check;
ALTER TABLE auth_pending ADD CONSTRAINT auth_pending_purpose_check
  CHECK (purpose IN ('TOTP', 'ENROLL', 'RESET', 'PASSWORD', 'LINK', 'CONNECT', 'ADDRESS', 'TERMS',
                     'PASSKEY_REGISTER', 'PASSKEY_SIGN_IN'));

-- No account exactly where a TERMS arrival or a passkey sign-in's challenge stands.
ALTER TABLE auth_pending DROP CONSTRAINT IF EXISTS auth_pending_arrival_whole;
ALTER TABLE auth_pending ADD CONSTRAINT auth_pending_arrival_whole
  CHECK (CASE WHEN purpose = 'PASSKEY_SIGN_IN'
              THEN account_id IS NULL AND arrival_email IS NULL
              ELSE (account_id IS NULL) = (arrival_email IS NOT NULL) END);

ALTER TABLE auth_pending DROP CONSTRAINT IF EXISTS auth_pending_arrival_terms;
ALTER TABLE auth_pending ADD CONSTRAINT auth_pending_arrival_terms
  CHECK (account_id IS NOT NULL OR purpose = 'PASSKEY_SIGN_IN'
         OR (purpose = 'TERMS' AND link_provider_id IS NOT NULL));

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
