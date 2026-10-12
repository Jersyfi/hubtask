-- Managed accounts: a sign-in name instead of an address (ADR-0074, identity.md §13).
--
-- `sign_in_name` is what a managed account signs in with: stored folded (compatibility form, lower
-- case), unique in the workspace among accounts not deleted, never containing `@` - so an
-- identifier is an address or a name by its shape alone. `start_password_at` says the password in
-- `password_hash` was drawn by Hubtask rather than chosen by the person, and when; the first sign-in
-- with it meets the change step, and every door that sets a password clears it.
--
-- `auth_pending.password_change_owed` carries an owed change across a factor step: a sign-in whose
-- password the rule refused, or a start password, still meets the change after its code or its
-- enrolment (identity.md §4.2).
--
-- Expand only: three nullable or defaulted columns and an index. A pod of the previous release
-- neither reads nor writes them (rule 12).
-- +goose Up
ALTER TABLE account ADD COLUMN IF NOT EXISTS sign_in_name text
  CONSTRAINT account_sign_in_name_check
  CHECK (char_length(sign_in_name) BETWEEN 3 AND 32 AND position('@' IN sign_in_name) = 0);
ALTER TABLE account ADD COLUMN IF NOT EXISTS start_password_at timestamptz;

-- Not CONCURRENTLY: a migration runs in a transaction, and every value is NULL when it is built, so
-- the partial index starts empty.
CREATE UNIQUE INDEX IF NOT EXISTS account_sign_in_name_uq ON account (tenant_id, sign_in_name)
  WHERE sign_in_name IS NOT NULL AND deleted_at IS NULL;

-- A constant default is stored in the catalogue, not written into every row.
ALTER TABLE auth_pending ADD COLUMN IF NOT EXISTS password_change_owed boolean NOT NULL DEFAULT false;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
