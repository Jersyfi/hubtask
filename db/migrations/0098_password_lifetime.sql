-- The password gets a life (ADR-0068 §3, §5, §6).
--
-- Until now a password was set once, at redemption, and nothing else was ever true about it. Four
-- things have to be, for a rule an installation can change to mean anything: when it was set, what
-- it was before, that a sign-in can be answered with "set a new one", and that a mailbox can prove
-- enough to be allowed to.
--
-- `password_set_at` is backfilled for accounts that have a password, from the row's own last write.
-- That is an estimate and is meant as one: without it `max_age_days` would be a switch that
-- silently did nothing for every account that existed before it, which is worse than a date that
-- is a few hours out. An account with no password gets no moment, because there is nothing to date.
--
-- `account_password_history` holds hashes and nothing else - no plaintext, no hint about the shape
-- of one - and is capped at ten by the rule rather than by the schema, because the cap is a policy
-- value and a constraint would have to move with it. It goes with the account, by cascade.
--
-- `auth_pending.purpose` gains two: `RESET` is the token a reset mail carries, `PASSWORD` is the
-- credential the PASSWORD_CHANGE step of a sign-in hands out. Both are the same discipline the
-- second factor's pending credential already has - hashed under its own purpose label, minutes
-- long, single use - which is why neither needs a table of its own.
--
-- `session.signed_in_with` records how a session was opened, for the list a person reads and for
-- the day a passkey is one of the answers. NULL is a session opened before this column existed.
--
-- Expand only: one nullable column with a backfill, one new table, one widened check constraint,
-- one more nullable column. Nothing a running older version writes becomes invalid.

-- +goose Up
ALTER TABLE account ADD COLUMN IF NOT EXISTS password_set_at timestamptz;
UPDATE account
   SET password_set_at = coalesce(updated_at, created_at)
 WHERE password_hash IS NOT NULL
   AND password_set_at IS NULL;

CREATE TABLE IF NOT EXISTS account_password_history (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id    uuid NOT NULL,
  password_hash text NOT NULL,
  set_at        timestamptz NOT NULL,
  CONSTRAINT account_password_history_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
-- The read is always "this account's, newest first", and the write trims by the same order.
CREATE INDEX IF NOT EXISTS account_password_history_account_idx
  ON account_password_history (tenant_id, account_id, set_at DESC);

ALTER TABLE account_password_history ENABLE ROW LEVEL SECURITY;
ALTER TABLE account_password_history FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON account_password_history
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON account_password_history TO hubtask_app;

ALTER TABLE auth_pending DROP CONSTRAINT IF EXISTS auth_pending_purpose_check;
ALTER TABLE auth_pending ADD CONSTRAINT auth_pending_purpose_check
  CHECK (purpose IN ('TOTP', 'ENROLL', 'RESET', 'PASSWORD'));

ALTER TABLE session ADD COLUMN IF NOT EXISTS signed_in_with text;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
