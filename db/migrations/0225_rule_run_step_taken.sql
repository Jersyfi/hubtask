-- A run a workflow step started (UC-FLW-04 check 4, automation.md §1.1): the closed set of
-- `rule_run.trigger` takes STEP_TAKEN.
--
-- Safe for a rolling update: only a widening. An older binary never writes the new value, and a
-- newer one writing it beside an older one breaks nothing the older one reads. Added NOT VALID -
-- a partitioned table takes one and passes it to every partition - and validated in 0335, outside
-- a transaction. Drop and add are one transaction, so no moment holds neither check.

-- +goose Up

ALTER TABLE rule_run DROP CONSTRAINT IF EXISTS rule_run_trigger_check;
ALTER TABLE rule_run
  ADD CONSTRAINT rule_run_trigger_check
  CHECK (trigger IN ('EVENT','SCHEDULE','RELATIVE_DATE','INBOUND_WEBHOOK','MANUAL','JUMBLE_ENTRY','STEP_TAKEN')) NOT VALID;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
