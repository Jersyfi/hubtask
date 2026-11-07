-- A product home's first visit points once at where to create there (user-experience.md §3.4), once
-- per person: the products whose mark the person has seen are the account's, beside
-- onboarding_completed_at, so a second device does not show them again.
--
-- Expand only (rule 12): one column with a constant default, which writes no row, and a bound on
-- the list added NOT VALID, so it takes no scan under the lock; every row holds the empty default.
-- A pod of the previous release never reads the column and writes preferences as before.
-- +goose Up
ALTER TABLE account ADD COLUMN IF NOT EXISTS products_seen text[] NOT NULL DEFAULT '{}';

-- +goose StatementBegin
DO $bound$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'account_products_seen_bound') THEN
    ALTER TABLE account ADD CONSTRAINT account_products_seen_bound
      CHECK (cardinality(products_seen) <= 9) NOT VALID;
  END IF;
END
$bound$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
