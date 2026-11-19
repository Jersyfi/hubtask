-- How a person arranged *Today*'s cards and tiles (work-surface concept E-17): the order and the
-- hidden ones, the account's as the sidebar's are, so they follow the person to another device.
--
-- Expand only (rule 12): two lists with a constant default, which writes no row, and their bound
-- added NOT VALID, so it takes no scan under the lock; 0397 validates it. A pod of the previous
-- release never reads the columns and writes the preferences as before.
-- +goose Up
ALTER TABLE account ADD COLUMN IF NOT EXISTS today_sections text[] NOT NULL DEFAULT '{}';
ALTER TABLE account ADD COLUMN IF NOT EXISTS today_sections_hidden text[] NOT NULL DEFAULT '{}';

-- +goose StatementBegin
DO $bound$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'account_today_sections_bound') THEN
    ALTER TABLE account ADD CONSTRAINT account_today_sections_bound
      CHECK (cardinality(today_sections) <= 16 AND cardinality(today_sections_hidden) <= 16) NOT VALID;
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
