-- The occasion a run answered, on the run itself (G-09).
--
-- The idempotency key of every action is (rule, occasion, action path), and the occasion is what
-- made the run one occurrence: the event, a schedule's instant, a manual press. Until now it
-- lived only in the job payload, which the queue deletes with the finished job - so a replay of a
-- half-finished run could not reconstruct the keys its finished actions claimed, and would repeat
-- them instead of completing around them.
--
-- Forward-only and safe for a rolling update: one nullable column, added without a table rewrite.
-- Rows written before this release stay NULL, and the replay derives the occasion where the kind
-- makes that possible (an event's run) and refuses honestly where it does not.

-- +goose Up

ALTER TABLE rule_run
  ADD COLUMN IF NOT EXISTS occasion text;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
