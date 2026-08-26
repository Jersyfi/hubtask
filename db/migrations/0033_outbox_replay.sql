-- +goose Up
-- backup-restore.md §8.4 forbids a restore from firing automation: "restored changes produce
-- events with `replay: true`, which the rule engine ignores". The flag arrives with E-06, which is
-- the task that builds the restore, rather than with the rule engine in `0.5.0` - an engine that
-- had to be taught the rule after the fact would be an engine with a window in which it did not
-- know it.
--
-- It is a column rather than a key in the payload because it is routing: the dispatcher decides
-- what to hand a subscriber before anything parses `data`, and a decision that needed the payload
-- would be a decision every subscriber made for itself.
--
-- Expand only. FALSE for everything already written, which is what every one of them was.
ALTER TABLE outbox_event ADD COLUMN replay boolean NOT NULL DEFAULT false;

-- +goose Down
-- Development only; in production the rule is to fix forwards (versioning-release.md §4).
ALTER TABLE outbox_event DROP COLUMN replay;
