-- A notification category for the team's public answer on a portal request (UC-SVC-05 check 9):
-- the closed set of `notification.category` and `notification_preference.category` takes
-- REQUEST_REPLY.
--
-- Safe for a rolling update: only a widening. An older binary never writes the new value; no
-- released binary runs the portal (migration 0240 and its successors are not on a release), so
-- none reads a requester's record. Drop and add are one transaction, so no moment holds neither
-- check.

-- +goose Up

ALTER TABLE notification DROP CONSTRAINT notification_category_check;
ALTER TABLE notification ADD CONSTRAINT notification_category_check
  CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER','INTEGRATION','RETENTION','EMERGENCY_ACCESS','REQUEST_REPLY'));

ALTER TABLE notification_preference DROP CONSTRAINT notification_preference_category_check;
ALTER TABLE notification_preference ADD CONSTRAINT notification_preference_category_check
  CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER','INTEGRATION','RETENTION','EMERGENCY_ACCESS','REQUEST_REPLY'));

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
