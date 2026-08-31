-- The proof before the irreversible (H-03, security.md §5): a step-up is recorded on the current
-- session - which is the whole point, "a fresh re-authentication on the current session" - so
-- the columns live there rather than in a table of their own. One live proof per session: a
-- second privileged action needs a second proof, and a row that could hold two would be a row
-- that could cover two.
--
-- The token is stored as a hash under its own purpose label, D-08's discipline; the method is
-- recorded for the audit trail, never the credential.

-- Forward-only and safe for a rolling update: four nullable columns, which PostgreSQL adds
-- without rewriting the table, and an index built CONCURRENTLY outside a transaction.

-- +goose NO TRANSACTION

-- +goose Up

ALTER TABLE session
  ADD COLUMN IF NOT EXISTS step_up_token_hash bytea,
  ADD COLUMN IF NOT EXISTS step_up_at timestamptz,
  ADD COLUMN IF NOT EXISTS step_up_method text,
  ADD COLUMN IF NOT EXISTS step_up_consumed_at timestamptz;

-- The lookup the consuming action makes. Partial, because most sessions never step up; unique,
-- because the hash covers the whole presented string, tenant half included.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS session_step_up_token_uq
  ON session (step_up_token_hash)
  WHERE step_up_token_hash IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
