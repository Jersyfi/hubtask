-- A retention rule whose condition cannot be evaluated on a candidate is set aside for that pass,
-- and shows why until a pass evaluates it cleanly (data-retention.md §2, decision of 2026-10-15).
-- The code and the moment only: never the condition's text or an entry's value (rule 10). Both or
-- neither.
--
-- Forward-only and safe for a rolling update: two nullable columns, which a pod of the previous
-- release neither reads nor writes.

-- +goose Up
ALTER TABLE retention_rule
  ADD COLUMN last_failure_code text,
  ADD COLUMN last_failure_at   timestamptz,
  ADD CONSTRAINT retention_rule_last_failure_whole
    CHECK ((last_failure_code IS NULL) = (last_failure_at IS NULL));

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
