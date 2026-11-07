-- The second half of 0372: the widened key CHECK validated without a transaction, so it takes
-- only a lock writes pass.

-- +goose NO TRANSACTION

-- +goose Up
ALTER TABLE config_draft VALIDATE CONSTRAINT config_draft_key_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
