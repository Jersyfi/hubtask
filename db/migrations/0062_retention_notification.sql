-- The retention advance warning becomes a notification anybody can receive (R-1, G-12).
--
-- data-retention.md §6 has asked for two kinds of visibility since the first draft: the object
-- carries what is coming, and the people who can stop it are told. The first was built with the
-- marking; the second was refused at the door - a rule that asked to warn somebody was refused
-- rather than stored, because a configuration nothing enforces looks like a working installation
-- until the day somebody is waiting for the warning.
--
-- This is the column that lets it be stored: the category a retention warning is written under.
-- Its own category rather than a shade of REMINDER, because the switch is a different switch -
-- somebody who has silenced every other message still needs the one about work that is about to
-- stop existing, and the window in which they can answer it is the grace period.
--
-- Safe for a rolling update: widening a CHECK constraint accepts everything it accepted before,
-- and an old binary never writes the new value.

-- +goose Up

ALTER TABLE notification DROP CONSTRAINT notification_category_check;
ALTER TABLE notification ADD CONSTRAINT notification_category_check
  CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER','INTEGRATION','RETENTION'));

-- The preference table carries the same closed set: a category nobody can switch off would be a
-- notification with no preference behind it, which is the one thing data-protection.md §9 asks
-- against.
ALTER TABLE notification_preference DROP CONSTRAINT notification_preference_category_check;
ALTER TABLE notification_preference ADD CONSTRAINT notification_preference_category_check
  CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER','INTEGRATION','RETENTION'));

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
