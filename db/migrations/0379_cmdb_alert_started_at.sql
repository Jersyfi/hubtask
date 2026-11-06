-- When the monitoring program says an alert's current episode began (Alertmanager's startsAt), so
-- that a firing retried after its resolution - which still carries the start of the episode that
-- resolved - reopens nothing (UC-AST-08).
--
-- Expand only: one nullable column; the previous version neither writes nor reads it, and an alert
-- it writes keeps the value standing, which only makes a stale firing compare with an older start.

-- +goose Up

ALTER TABLE cmdb_alert ADD COLUMN IF NOT EXISTS started_at timestamptz;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
