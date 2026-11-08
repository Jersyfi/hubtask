-- Validates 0387's bound on the sidebar's preferences: every row holds the defaults or what the
-- client wrote within them. Without a transaction, under a lock writes pass.

-- +goose NO TRANSACTION

-- +goose Up
ALTER TABLE account VALIDATE CONSTRAINT account_nav_bound;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
