-- The index a hook's removal cascades through, and the validation of 0346's constraints, outside a
-- transaction so no write waits behind the scan (versioning-release.md §4).
-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS notification_code_hook_idx
  ON notification (tenant_id, code_hook_id) WHERE code_hook_id IS NOT NULL;

ALTER TABLE notification VALIDATE CONSTRAINT notification_code_hook_id_fkey;
ALTER TABLE notification VALIDATE CONSTRAINT notification_one_subject_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
