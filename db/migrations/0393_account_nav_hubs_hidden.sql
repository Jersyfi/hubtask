-- The hubs a person moved behind *More* in the sidebar (navigation concept §7.5, the owner's review
-- R-4): the account's, as the hidden products are, so they follow the person to another device.
--
-- Expand only (rule 12): a list with a constant default, which writes no row, and its bound added
-- NOT VALID, so it takes no scan under the lock; 0394 validates it. A pod of the previous release
-- never reads the column and writes the preferences as before.
-- +goose Up
ALTER TABLE account ADD COLUMN IF NOT EXISTS nav_hubs_hidden uuid[] NOT NULL DEFAULT '{}';

-- +goose StatementBegin
DO $bound$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'account_nav_hubs_hidden_bound') THEN
    ALTER TABLE account ADD CONSTRAINT account_nav_hubs_hidden_bound
      CHECK (cardinality(nav_hubs_hidden) <= 1000) NOT VALID;
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
