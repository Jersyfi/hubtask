-- Validates 0403's widened category checks on `notification` and `notification_preference`: every
-- row holds one of the closed set, POST included. Without a transaction, under a lock writes pass.

-- +goose NO TRANSACTION

-- +goose Up
ALTER TABLE notification VALIDATE CONSTRAINT notification_category_check;
ALTER TABLE notification_preference VALIDATE CONSTRAINT notification_preference_category_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
