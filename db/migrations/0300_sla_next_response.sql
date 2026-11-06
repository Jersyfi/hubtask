-- An SLA goal for the next response (UC-SVC-02 check 8): the closed set of `entry_sla.goal` takes
-- NEXT_RESPONSE. A policy's goals are jsonb and read without a check, so only the record's set
-- widens.
--
-- Safe for a rolling update: only a widening. An older binary never writes the new value; one that
-- reads a policy naming it arms it as a goal of its own kind (the store maps a kind without
-- validating it), which no released binary does, since none before this one keeps SLA records.
-- Drop and add are one transaction, so no moment holds neither check.

-- +goose Up

ALTER TABLE entry_sla DROP CONSTRAINT IF EXISTS entry_sla_goal_check;
ALTER TABLE entry_sla
  ADD CONSTRAINT entry_sla_goal_check
  CHECK (goal IN ('FIRST_RESPONSE', 'NEXT_RESPONSE', 'RESOLUTION'));

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
