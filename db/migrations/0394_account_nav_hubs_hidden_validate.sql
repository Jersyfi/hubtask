-- Validates 0393's bound on the hidden hubs. Without a transaction, under a lock writes pass.

-- +goose NO TRANSACTION

-- +goose Up
ALTER TABLE account VALIDATE CONSTRAINT account_nav_hubs_hidden_bound;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
