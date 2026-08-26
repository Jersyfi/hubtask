-- +goose Up
-- The role audit.md §5 has named since the document was written, and that no installation could
-- grant: an `AUDITOR` reads the audit trail and the configuration and no content at all.
--
-- It exists because the alternative, in practice, is giving the auditor administrator rights - a
-- permissions problem that arises precisely where evidence is being demanded. Somebody who has to
-- read the workspace as well holds a second membership; the rights add up rather than the stronger
-- one winning, which is what the application layer's union over memberships does
-- (core/domain/service/Authorization.go).
--
-- Expand only, and an enum value is the cheapest expansion there is: nothing already stored means
-- anything different afterwards. `AUDITOR` is appended rather than sorted into place, because the
-- stored order of an enum is what comparisons and indexes use, and rewriting it would rewrite
-- every membership row for a value none of them holds.
ALTER TYPE membership_role ADD VALUE IF NOT EXISTS 'AUDITOR';

-- +goose Down
-- Nothing. PostgreSQL cannot remove a value from an enum, and the way back would be to build a new
-- type, move every column onto it and drop the old one - a rewrite of the membership table to undo
-- a value nobody was granted. Development only in any case; in production the rule is to fix
-- forwards (versioning-release.md §4).
SELECT 1;
