-- The authenticator's replacement waits beside the armed factor (SC-17).
--
-- Replacing an authenticator - a new phone, a lost app - used to mean turning the factor off and
-- setting one up again, a moment with no factor that a workspace requiring one forbids. The new
-- secret is kept here, sealed, as a second and unconfirmed enrolment beside the armed one, and it
-- becomes the armed one in a single statement once a code from the new app confirms it: the old
-- factor works until that statement and not after it.
--
-- The session that began it and the end of its window are stored with it, because only that
-- session may confirm it and only for a few minutes. No foreign key on the session, for migration
-- 0111's reason: a session ending is not a reason to touch this row, and a confirmation from any
-- other session is refused by the statement anyway.
--
-- Four columns that are all present or all absent, which the check says rather than the code.
-- +goose Up
ALTER TABLE account_mfa
  ADD COLUMN IF NOT EXISTS replacement_secret_enc    bytea,
  ADD COLUMN IF NOT EXISTS replacement_secret_key_id text,
  ADD COLUMN IF NOT EXISTS replacement_session_id    uuid,
  ADD COLUMN IF NOT EXISTS replacement_expires_at    timestamptz;
ALTER TABLE account_mfa DROP CONSTRAINT IF EXISTS account_mfa_replacement_whole;
ALTER TABLE account_mfa ADD CONSTRAINT account_mfa_replacement_whole CHECK (
  (replacement_secret_enc IS NULL) = (replacement_secret_key_id IS NULL)
  AND (replacement_secret_enc IS NULL) = (replacement_session_id IS NULL)
  AND (replacement_secret_enc IS NULL) = (replacement_expires_at IS NULL)
);

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
