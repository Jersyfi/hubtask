-- Validates 0432's checks and foreign key on `notification` and `notification_preference`, and
-- indexes the new subject. Without a transaction, so writes pass while it runs.

-- +goose NO TRANSACTION

-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS notification_cmdb_campaign_idx
  ON notification (tenant_id, cmdb_campaign_id) WHERE cmdb_campaign_id IS NOT NULL;

ALTER TABLE notification VALIDATE CONSTRAINT notification_category_check;
ALTER TABLE notification_preference VALIDATE CONSTRAINT notification_preference_category_check;
ALTER TABLE notification VALIDATE CONSTRAINT notification_cmdb_campaign_id_fkey;
ALTER TABLE notification VALIDATE CONSTRAINT notification_one_subject_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
