-- A webhook subscription can be narrowed to one hub or one collection, and an event its condition
-- could not be evaluated for is kept in the delivery log (UC-INT-01 check 9, automation.md §3.1).
--
-- The scope is the rules' shape: a level and an identifier, both absent for the whole workspace -
-- which every subscription before this column is. The delivery's new status is a row nothing was
-- sent for. Expand-only: the previous binary writes neither and reads the status only to list it.
-- The check is replaced NOT VALID and then validated, so the table is not held while it is read.
-- +goose Up
ALTER TABLE webhook_subscription ADD COLUMN IF NOT EXISTS scope_type text;
ALTER TABLE webhook_subscription ADD COLUMN IF NOT EXISTS scope_id uuid;
ALTER TABLE webhook_subscription DROP CONSTRAINT IF EXISTS webhook_subscription_scope_check;
ALTER TABLE webhook_subscription ADD CONSTRAINT webhook_subscription_scope_check CHECK (
  (scope_type IS NULL AND scope_id IS NULL)
  OR (scope_type IN ('HUB','COLLECTION') AND scope_id IS NOT NULL)
) NOT VALID;
ALTER TABLE webhook_subscription VALIDATE CONSTRAINT webhook_subscription_scope_check;

ALTER TABLE webhook_delivery DROP CONSTRAINT IF EXISTS webhook_delivery_status_check;
ALTER TABLE webhook_delivery ADD CONSTRAINT webhook_delivery_status_check
  CHECK (status IN ('PENDING','SUCCEEDED','FAILED','DEAD_LETTER','FILTER_FAILED')) NOT VALID;
ALTER TABLE webhook_delivery VALIDATE CONSTRAINT webhook_delivery_status_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
