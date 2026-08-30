-- When a SCHEDULE rule next fires (G-08, automation.md §1.1, decision 5 of milestone-0.5.0).
--
-- The moment is stored rather than derived on every pass, for the reason `backup_schedule` stores
-- one (E-05): a poller that re-expanded every rule's RRULE would pay a library call for every rule
-- that is not due, and the expansion is also where a rule this installation cannot read is refused -
-- at the moment somebody writes it, rather than at three in the morning.
--
-- The index is `backup_schedule_due_idx`'s shape deliberately: the tenant predicate is row level
-- security's, added to every read, and the ordering the pass wants is by moment. Partial on the two
-- states a poller never asks about - a disabled rule and a deleted one - so a workspace's switched
-- off rules cost no index entry each.

-- Forward-only and safe for a rolling update: one nullable column, which PostgreSQL adds without
-- rewriting the table, and an index built CONCURRENTLY outside a transaction.

-- +goose NO TRANSACTION

-- +goose Up

ALTER TABLE automation_rule
  ADD COLUMN IF NOT EXISTS next_run_at timestamptz;

-- NULL is the honest value for the five triggers that are not a schedule, and also for a schedule
-- whose rule is exhausted - `FREQ=YEARLY;UNTIL=…` that has passed its last occurrence. A rule with
-- no next moment is stored with none rather than refused: the rule may be perfectly good and simply
-- over, and one an operator can see and edit is better than an error that loses what they typed.
CREATE INDEX CONCURRENTLY IF NOT EXISTS automation_rule_due_idx
  ON automation_rule (next_run_at)
  WHERE enabled AND deleted_at IS NULL AND next_run_at IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
