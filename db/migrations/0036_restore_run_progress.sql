-- +goose Up
-- BK-7 asks that a restore interrupted by process death resumes without duplicates. For SKIP and
-- OVERWRITE that is free: everything a restore writes is keyed by identity, so re-applying a record
-- that is already there changes nothing. For DUPLICATE it is not - the question the rule turns on,
-- "does the tenant already hold this", changes its answer once the first attempt has written half
-- the archive, and the second attempt would duplicate what the first one merely restored.
--
-- So a restore records how far it got, per entity, in the same transaction as the batch it got
-- there with. A resumed attempt skips what has already been decided; the archive is immutable and
-- the read order is fixed, so "the first N records of this entity" names the same N records on
-- every attempt.
--
-- The report is written with it, in the same statement, because a resumed attempt has to continue
-- counting rather than start again - a report that only covered the last attempt would say a
-- restore did a fraction of what it did.
--
-- Expand only. NULL for everything already written, which is what "nothing has been decided yet"
-- looks like.
ALTER TABLE restore_run ADD COLUMN progress jsonb;

-- +goose Down
-- Development only; in production the rule is to fix forwards (versioning-release.md §4).
ALTER TABLE restore_run DROP COLUMN progress;
