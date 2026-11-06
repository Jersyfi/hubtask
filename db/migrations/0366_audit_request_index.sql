-- Support looks one request up in the trail by its id (Audit.sql, ListAuditEntries' request_id).
-- The index is created ON ONLY the parent, so this file builds nothing and holds no partition: a
-- partition created later takes it at once, and the migrator builds it on every partition already
-- there CONCURRENTLY and attaches it (cmd/migrate attachPartitionIndexes), after which the parent's
-- index is valid. Expand only (rule 12).

-- +goose Up

CREATE INDEX IF NOT EXISTS audit_request_idx
  ON ONLY audit_log (tenant_id, (context->>'RequestID'), occurred_at DESC);

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
