-- +goose Up
-- data-retention.md §6: "Retention nobody can see will eventually surprise somebody." The
-- announcement an object carries says what is coming; what it could not say until now is that
-- something is *stopping* it - and "this would have been deleted, and a legal hold is holding it
-- back" is exactly what somebody looking at the object needs to know. The contract has carried
-- `RetentionState.blocked_by` since A-06 against a column that did not exist.
--
-- Its own column rather than a value of `retention_action`, because the two are different
-- statements: the action is what the rule would do and the block is why it is not happening. A
-- blocked object carries the rule and the action and *no* `retention_pending_until`, which is what
-- keeps phase two from acting on something that is being held back - the absence of a date is the
-- absence of a due moment rather than a flag anybody has to remember to check.
--
-- Expand only. NULL for everything already written, which is what "nothing is stopping it" looks
-- like.
ALTER TABLE work_item ADD COLUMN IF NOT EXISTS retention_blocked_by text;

-- +goose Down
-- Development only; in production the rule is to fix forwards (versioning-release.md §4).
ALTER TABLE work_item DROP COLUMN retention_blocked_by;
