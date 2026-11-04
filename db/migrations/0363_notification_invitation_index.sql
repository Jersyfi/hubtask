-- The people list says whether the last invitation mail actually went: the queue's invitation job
-- only records the notification and queues its delivery, so whether the mail was sent, failed or
-- was suppressed is the newest INVITATION notification of the account. Nothing indexes a
-- notification by its recipient, so this read would scan the workspace's notifications. Built
-- CONCURRENTLY without a transaction, as 0362; a leftover invalid index is dropped first.

-- +goose NO TRANSACTION

-- +goose Up

-- +goose StatementBegin
DO $leftover$
BEGIN
  SET LOCAL lock_timeout = '5s';
  IF EXISTS (SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
             WHERE NOT i.indisvalid AND c.relname = 'notification_invitation_idx') THEN
    DROP INDEX IF EXISTS notification_invitation_idx;
  END IF;
END
$leftover$;
-- +goose StatementEnd

CREATE INDEX CONCURRENTLY IF NOT EXISTS notification_invitation_idx
  ON notification (tenant_id, recipient_id, created_at DESC) WHERE category = 'INVITATION';

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
