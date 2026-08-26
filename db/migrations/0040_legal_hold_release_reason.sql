-- +goose Up
-- `legal_hold` carries `released_by` and `released_at` since `0001_init` and no reason for either.
-- data-retention.md §4.1 makes lifting a hold auditable, and the moment the data under it becomes
-- deletable again is the moment an auditor most needs to know *why* somebody decided that -
-- "released" with no reason is an entry nobody can act on.
--
-- The reason for *placing* one has always been there, which is what makes the absence of this one
-- an oversight rather than a decision: both ends of a hold are decisions, and only one of them was
-- recorded.
--
-- Expand only. NULL for the holds released before this existed, which is what "nobody wrote one
-- down" looks like.
ALTER TABLE legal_hold ADD COLUMN IF NOT EXISTS released_reason text;

-- +goose Down
-- Development only; in production the rule is to fix forwards (versioning-release.md §4).
ALTER TABLE legal_hold DROP COLUMN released_reason;
