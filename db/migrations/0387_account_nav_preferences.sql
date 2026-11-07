-- The sidebar's personal preferences (user-experience.md §2.3, navigation concept §7.5): the
-- products' order and the ones behind *More*, how the hubs and a hub's contents are ordered. The
-- account's, so they follow the person to another device; the width and the rail stay the device's.
--
-- Expand only (rule 12): two lists with a constant default and two nullable words, which write no
-- row, and their bounds added NOT VALID, so they take no scan under the lock; 0388 validates them.
-- A pod of the previous release never reads the columns and writes the preferences as before.
-- +goose Up
ALTER TABLE account ADD COLUMN IF NOT EXISTS nav_products text[] NOT NULL DEFAULT '{}';
ALTER TABLE account ADD COLUMN IF NOT EXISTS nav_products_hidden text[] NOT NULL DEFAULT '{}';
ALTER TABLE account ADD COLUMN IF NOT EXISTS nav_hubs_order text;
ALTER TABLE account ADD COLUMN IF NOT EXISTS nav_contents_mode text;

-- +goose StatementBegin
DO $bound$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'account_nav_bound') THEN
    ALTER TABLE account ADD CONSTRAINT account_nav_bound CHECK (
      cardinality(nav_products) <= 9 AND cardinality(nav_products_hidden) <= 9
      AND (nav_hubs_order IS NULL OR nav_hubs_order IN ('MANUAL', 'RECENT', 'NAME'))
      AND (nav_contents_mode IS NULL OR nav_contents_mode IN ('TREE', 'RECENT', 'NAME'))
    ) NOT VALID;
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
