-- A feed's kind (UC-INT-15): a calendar, as every feed has been, or a read-only data feed of the
-- view's rows for a reporting tool. One token opens one kind, so a calendar address kept on a
-- phone never reads a view's fields.
--
-- Expand only: a column with a default every older reader ignores and every older writer fills
-- (rule 12). The check is added NOT VALID and validated in 0328, outside a transaction: inline on
-- the column it would scan the table under the ALTER's exclusive lock.
-- +goose Up
ALTER TABLE calendar_feed
  ADD COLUMN IF NOT EXISTS kind text NOT NULL DEFAULT 'CALENDAR';
ALTER TABLE calendar_feed DROP CONSTRAINT IF EXISTS calendar_feed_kind_check,
  ADD CONSTRAINT calendar_feed_kind_check CHECK (kind IN ('CALENDAR', 'DATA')) NOT VALID;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
