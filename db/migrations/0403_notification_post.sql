-- A notification category for posts (UC-DOC-17, UC-DOC-18): a post reaching its audience and a
-- writer's reminder to confirm it. The closed set of `notification.category` and
-- `notification_preference.category` takes POST.
--
-- Safe for a rolling update: only a widening (rule 12). An older binary never writes the new value
-- and reads a category it does not know only through a preference list, which answers the stored
-- value as it is. Drop and add are one transaction, so no moment holds neither check.

-- +goose Up

ALTER TABLE notification DROP CONSTRAINT notification_category_check;
ALTER TABLE notification ADD CONSTRAINT notification_category_check
  CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER','INTEGRATION','RETENTION','EMERGENCY_ACCESS','REQUEST_REPLY','SLA','POST')) NOT VALID;

ALTER TABLE notification_preference DROP CONSTRAINT notification_preference_category_check;
ALTER TABLE notification_preference ADD CONSTRAINT notification_preference_category_check
  CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER','INTEGRATION','RETENTION','EMERGENCY_ACCESS','REQUEST_REPLY','SLA','POST')) NOT VALID;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
