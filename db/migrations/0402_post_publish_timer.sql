-- A scheduled post is published by a durable timer (work-surface concept B-3): the timer pass reads
-- the clock and fires the POST_PUBLISH purpose's handler at the post's moment, as PAGE_REVIEW does
-- for a page's review (migration 0239).
--
-- Expand only (rule 12): one more value the constraint admits; nothing older writes it.
-- +goose Up
ALTER TABLE durable_timer DROP CONSTRAINT IF EXISTS durable_timer_purpose_check;
ALTER TABLE durable_timer ADD CONSTRAINT durable_timer_purpose_check
  CHECK (purpose IN ('SLA', 'APPROVAL', 'EFFECTIVENESS_CHECK', 'PAGE_REVIEW', 'OLA', 'POST_PUBLISH')) NOT VALID;
ALTER TABLE durable_timer VALIDATE CONSTRAINT durable_timer_purpose_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
