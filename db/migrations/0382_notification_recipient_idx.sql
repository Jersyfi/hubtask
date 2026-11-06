-- The people list says how the last mail of any kind to a person went (user-experience.md §8.3),
-- not only the invitation's: the newest notification of the account, whatever its category, and its
-- delivery job's end and failure. The invitation index (0370) is partial and no index finds a
-- delivery job by its notification, so this read would scan the workspace's notifications and the
-- queue. Built CONCURRENTLY without a transaction, as 0370; a leftover invalid index is dropped
-- first.

-- +goose NO TRANSACTION

-- +goose Up

-- +goose StatementBegin
DO $leftover$
BEGIN
  SET LOCAL lock_timeout = '5s';
  IF EXISTS (SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
             WHERE NOT i.indisvalid AND c.relname = 'notification_recipient_idx') THEN
    DROP INDEX IF EXISTS notification_recipient_idx;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
             WHERE NOT i.indisvalid AND c.relname = 'job_notification_delivery_idx') THEN
    DROP INDEX IF EXISTS job_notification_delivery_idx;
  END IF;
END
$leftover$;
-- +goose StatementEnd

CREATE INDEX CONCURRENTLY IF NOT EXISTS notification_recipient_idx
  ON notification (tenant_id, recipient_id, created_at DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS job_notification_delivery_idx
  ON job (tenant_id, dedupe_key, created_at DESC) WHERE kind = 'notification.deliver';

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
