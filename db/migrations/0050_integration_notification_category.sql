-- +goose Up
-- A category for what an integration has to tell its owner (G-03, automation.md §3.1).
--
-- "Auto-disable after sustained unreachability, plus a notification to the owner" is one sentence,
-- and the second half of it had no category to be sent under: the closed set is ASSIGNMENT,
-- MEMBERSHIP, COMMENT, INVITATION and REMINDER, and a disabled webhook is none of those. Sending it
-- as one of them would be worse than not sending it - somebody who switched off COMMENT would stop
-- being told that their integration is broken.
--
-- Its own category rather than a shade of an existing one, on CategoryReminder's reasoning: the
-- switch is a different switch. Somebody who does not want to hear about comments still wants to
-- hear that the system stopped calling their server.
--
-- Both tables, because the preference carries the same closed set as the record.
--
-- Expand only: a widened CHECK accepts everything it accepted before, so an old pod writing the
-- five it knows is unaffected and a new pod writing the sixth is not refused by a constraint the
-- old one would have applied (rule 12, ADR-0003).
ALTER TABLE notification DROP CONSTRAINT IF EXISTS notification_category_check;
ALTER TABLE notification ADD CONSTRAINT notification_category_check
  CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER','INTEGRATION'));

ALTER TABLE notification_preference DROP CONSTRAINT IF EXISTS notification_preference_category_check;
ALTER TABLE notification_preference ADD CONSTRAINT notification_preference_category_check
  CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER','INTEGRATION'));

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
