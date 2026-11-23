-- Validates 0446's widened `comment_body_check`. Without a transaction, so writes pass while it
-- runs.

-- +goose NO TRANSACTION

-- +goose Up
ALTER TABLE comment VALIDATE CONSTRAINT comment_body_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
