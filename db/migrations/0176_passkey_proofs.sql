-- A passkey or security key as the second factor, the fresh proof and the setup during sign-in
-- (identity.md §23.3).
--
-- Each ceremony a known account answers gets a challenge row of its own purpose: PASSKEY_FACTOR for
-- the step after the password, PASSKEY_STEP_UP for a session's fresh proof - bound to that session -
-- and PASSKEY_ENROLL for a key set up during sign-in, which carries the user handle the new
-- credential will hold, as a registration's row does. Hashed under their own prefixes, five
-- minutes, single use.
--
-- Expand only: the previous release never writes the three purposes, and every row it writes still
-- passes both widened checks (rule 12).

-- +goose Up
ALTER TABLE auth_pending DROP CONSTRAINT IF EXISTS auth_pending_purpose_check;
ALTER TABLE auth_pending ADD CONSTRAINT auth_pending_purpose_check
  CHECK (purpose IN ('TOTP', 'ENROLL', 'RESET', 'PASSWORD', 'LINK', 'CONNECT', 'ADDRESS', 'TERMS',
                     'PASSKEY_REGISTER', 'PASSKEY_SIGN_IN', 'PASSKEY_FACTOR', 'PASSKEY_ENROLL',
                     'PASSKEY_STEP_UP'));

-- A session exactly on a registration's and a step-up's row, a user handle exactly on a
-- registration's and a setup's.
ALTER TABLE auth_pending DROP CONSTRAINT IF EXISTS auth_pending_passkey_whole;
ALTER TABLE auth_pending ADD CONSTRAINT auth_pending_passkey_whole
  CHECK ((session_id IS NOT NULL) = (purpose IN ('PASSKEY_REGISTER', 'PASSKEY_STEP_UP'))
         AND (user_handle IS NOT NULL) = (purpose IN ('PASSKEY_REGISTER', 'PASSKEY_ENROLL'))
         AND (user_handle IS NULL OR octet_length(user_handle) = 32));

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
