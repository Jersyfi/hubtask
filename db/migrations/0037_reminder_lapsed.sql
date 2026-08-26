-- +goose Up
-- backup-restore.md §8.4: "No reminders are caught up whose time lies in the past; they are marked
-- as lapsed." Until now there was no such mark. A restored reminder whose moment has gone would sit
-- PENDING with a `fire_at` in the past, and the scheduler's next pass would send every one of them
-- at once - which is the second of §8.4's four prohibitions, failed by an omission rather than by a
-- decision.
--
-- CANCELLED would have been the cheap answer and the wrong one: somebody cancelled that, and an
-- auditor reading a workspace after a restore would find hundreds of cancellations nobody made.
-- LAPSED says what happened - the moment passed while the data was in an archive.
--
-- Widening a check constraint is safe for a rolling update in both directions of the deployment:
-- an old process never writes the new value, and a new one never reads a row the old one refuses.
-- The two statements are one transaction, which is what goose gives them.
ALTER TABLE reminder DROP CONSTRAINT reminder_state_check;
ALTER TABLE reminder ADD CONSTRAINT reminder_state_check
  CHECK (state IN ('PENDING', 'SENT', 'CANCELLED', 'LAPSED'));

-- +goose Down
-- Development only; in production the rule is to fix forwards (versioning-release.md §4).
ALTER TABLE reminder DROP CONSTRAINT reminder_state_check;
ALTER TABLE reminder ADD CONSTRAINT reminder_state_check
  CHECK (state IN ('PENDING', 'SENT', 'CANCELLED'));
