-- What started a run, on the run itself (G-08, automation.md §1.1).
--
-- Until now every run was an event's, so the event identifier answered both "what started this"
-- and "what was it about". The other four triggers separate those two questions: a schedule starts
-- a run that no event caused, a relative date starts one about an entry nothing published, and a
-- manual trigger is the only kind a *person* pulls - which is the one the log has to name.
--
-- On the run rather than read back from the rule, because a rule can be edited from one kind into
-- another and a log that resolved the kind at read time would rewrite its own history.

-- Forward-only and safe for a rolling update: three nullable-or-defaulted columns, which PostgreSQL
-- adds without rewriting the table, and an index built CONCURRENTLY outside a transaction.

-- +goose NO TRANSACTION

-- +goose Up

-- 'EVENT' as the default is what every existing row already is: before G-08 there was no other way
-- for a run to start. A default rather than a backfill for that reason - there is nothing to guess.
ALTER TABLE rule_run
  ADD COLUMN IF NOT EXISTS trigger text NOT NULL DEFAULT 'EVENT',
  -- Who pulled it, for the one kind a person pulls. No foreign key: a run outlives the rule it
  -- belongs to by design, and it must outlive the account that started it for the same reason - a
  -- record of actions whose actor was deleted is still a record, and a cascade would erase it.
  ADD COLUMN IF NOT EXISTS triggered_by uuid,
  -- The entry the run is about when no event names it.
  ADD COLUMN IF NOT EXISTS subject_id uuid;

-- The closed set, added NOT VALID and validated separately: the first statement takes a brief lock
-- and no scan, the second scans without blocking writers. Every existing row carries the default
-- and passes, so the validation is a formality - but a formality that is checked rather than
-- assumed.
ALTER TABLE rule_run
  ADD CONSTRAINT rule_run_trigger_check
  CHECK (trigger IN ('EVENT','SCHEDULE','RELATIVE_DATE','INBOUND_WEBHOOK','MANUAL','JUMBLE_ENTRY'))
  NOT VALID;
ALTER TABLE rule_run VALIDATE CONSTRAINT rule_run_trigger_check;

-- What the listing's new filter asks: this tenant's runs of one kind, newest first. Partial on
-- nothing, because every run has a trigger - and leading with the tenant because that is the
-- predicate row level security adds to every read.
CREATE INDEX CONCURRENTLY IF NOT EXISTS rule_run_trigger_idx
  ON rule_run (tenant_id, trigger, id DESC);

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
