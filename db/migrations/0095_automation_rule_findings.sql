-- What the check found about a rule (ADR-0060, F8-03): every reference resolved against what
-- exists now, written on the rule so that reading the rules costs reading the rules. `findings` is
-- the list - empty for a rule with nothing wrong and for one never checked - and `checked_at`
-- tells those two apart.
--
-- Expand only: one column with a default every existing row means, one nullable column.

-- +goose Up
ALTER TABLE automation_rule
  ADD COLUMN findings jsonb NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN checked_at timestamptz;

-- +goose Down
ALTER TABLE automation_rule
  DROP COLUMN checked_at,
  DROP COLUMN findings;
