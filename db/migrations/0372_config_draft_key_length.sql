-- A configuration draft's key holds every key its block may have: a custom role's key is up to 63
-- characters (custom_role, shared.ValidKey), and 0315 held a draft's to 50, so a role with a
-- longer key could not be drafted.
--
-- Expand only (rule 12): the CHECK is widened, so whatever the previous release writes still
-- passes. It is added NOT VALID, without a scan under the lock, and validated in 0373.

-- +goose Up
ALTER TABLE config_draft DROP CONSTRAINT IF EXISTS config_draft_key_check,
  ADD CONSTRAINT config_draft_key_check CHECK (key ~ '^[a-z][a-z0-9_]{0,62}$') NOT VALID;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
