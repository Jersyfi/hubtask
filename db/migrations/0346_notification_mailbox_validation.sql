-- The index a mailbox's deletion cascades through, and the validation of 0345's constraints, outside
-- a transaction so no write waits behind the scan (versioning-release.md §4).
-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS notification_mailbox_idx
  ON notification (tenant_id, mailbox_id) WHERE mailbox_id IS NOT NULL;

ALTER TABLE notification VALIDATE CONSTRAINT notification_mailbox_id_fkey;
ALTER TABLE notification VALIDATE CONSTRAINT notification_one_subject_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
