-- A notification category for what a rule's notification step tells (UC-AUT-09): the closed set of
-- `notification.category` and `notification_preference.category` takes SLA.
--
-- Safe for a rolling update: only a widening. An older binary never writes the new value and reads
-- a category it does not know only through a preference list, which answers the stored value as
-- it is. Drop and add are one transaction, so no moment holds neither check.

-- +goose Up

ALTER TABLE notification DROP CONSTRAINT notification_category_check;
ALTER TABLE notification ADD CONSTRAINT notification_category_check
  CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER','INTEGRATION','RETENTION','EMERGENCY_ACCESS','REQUEST_REPLY','SLA'));

ALTER TABLE notification_preference DROP CONSTRAINT notification_preference_category_check;
ALTER TABLE notification_preference ADD CONSTRAINT notification_preference_category_check
  CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER','INTEGRATION','RETENTION','EMERGENCY_ACCESS','REQUEST_REPLY','SLA'));

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
