-- The people list reads how the last invitation mail to each account went: the newest job of the
-- invitation kind per account (its dedupe key), in one workspace. The queue's own indexes serve
-- only the pickup and the open dedupe keys, so a finished invitation would be a scan of the whole
-- queue. Built CONCURRENTLY without a transaction, so the queue's writes pass meanwhile; a
-- leftover invalid index of an interrupted build is dropped first, so a retry builds it again.

-- +goose NO TRANSACTION

-- +goose Up

-- +goose StatementBegin
DO $leftover$
BEGIN
  SET LOCAL lock_timeout = '5s';
  IF EXISTS (SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
             WHERE NOT i.indisvalid AND c.relname = 'job_invitation_delivery_idx') THEN
    DROP INDEX IF EXISTS job_invitation_delivery_idx;
  END IF;
END
$leftover$;
-- +goose StatementEnd

CREATE INDEX CONCURRENTLY IF NOT EXISTS job_invitation_delivery_idx
  ON job (tenant_id, dedupe_key, created_at DESC) WHERE kind = 'notification.invitation';

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
