-- The indexes a calendar push's or a connector's removal cascades through, and the validation of
-- 0342's constraints, outside a transaction so no write waits behind the scan
-- (versioning-release.md §4).
-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS notification_calendar_push_idx
  ON notification (tenant_id, calendar_push_id) WHERE calendar_push_id IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS notification_cmdb_connector_idx
  ON notification (tenant_id, cmdb_connector_id) WHERE cmdb_connector_id IS NOT NULL;

ALTER TABLE notification VALIDATE CONSTRAINT notification_calendar_push_id_fkey;
ALTER TABLE notification VALIDATE CONSTRAINT notification_cmdb_connector_id_fkey;
ALTER TABLE notification VALIDATE CONSTRAINT notification_one_subject_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
