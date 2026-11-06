-- A billing rate per client or project (UC-TIM-08): what an hour of billable time on a hub or a
-- collection is charged, from a day to a day or on. It is a rate as a cost rate is - an amount,
-- a currency and days, read only by whoever manages costs - so it is a row of `time_cost_rate`
-- whose subject is the plan in `plan_id`, never a person or a role.
--
-- Expand only (rule 12): a nullable column, and the subject check widened from "a person or a
-- role" to "exactly one of a person, a role or a plan". Every existing row keeps passing it. The
-- previous binary, until it is replaced, lists a plan's rate among the cost rates without a
-- subject, and prices no day by it: it matches a rate to a person or a role only.
-- +goose Up
ALTER TABLE time_cost_rate ADD COLUMN IF NOT EXISTS plan_id uuid;

-- +goose StatementBegin
DO $widen$
DECLARE
  old_check text;
BEGIN
  SELECT c.conname INTO old_check
  FROM pg_constraint c
  WHERE c.conrelid = 'time_cost_rate'::regclass AND c.contype = 'c'
    AND pg_get_constraintdef(c.oid) LIKE '%account_id IS NULL%role IS NULL%';
  IF old_check IS NOT NULL THEN
    EXECUTE format('ALTER TABLE time_cost_rate DROP CONSTRAINT %I', old_check);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'time_cost_rate_subject_check') THEN
    ALTER TABLE time_cost_rate ADD CONSTRAINT time_cost_rate_subject_check
      CHECK (num_nonnulls(account_id, role, plan_id) = 1);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'time_cost_rate_plan_fkey') THEN
    ALTER TABLE time_cost_rate ADD CONSTRAINT time_cost_rate_plan_fkey
      FOREIGN KEY (tenant_id, plan_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE;
  END IF;
END $widen$;
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS time_cost_rate_plan_idx ON time_cost_rate (tenant_id, plan_id, from_day)
  WHERE plan_id IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
