-- The account remembers its moments (F6-12, milestone-F6.md decision 11): whether the
-- celebrations of design-system.md §7 are marked for this person, and when the first-run tour
-- ended. Both are the account's own preferences, like its locale (ADR-0043 says why these are the
-- account's and the theme is not), and both are written by the client for itself.
--
-- Expand only: two nullable columns, no default written - NULL is "the default applies" for the
-- first and "the tour has not been taken" for the second, which is what every existing row means.

-- +goose Up
ALTER TABLE account
  ADD COLUMN celebrations boolean,
  ADD COLUMN onboarding_completed_at timestamptz;

-- +goose Down
ALTER TABLE account
  DROP COLUMN onboarding_completed_at,
  DROP COLUMN celebrations;
