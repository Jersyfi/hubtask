-- A notification about a forge hook that switched itself off: a delivery proved the hook's secret
-- while the person the hook acts as could no longer write in its hub, so every later delivery would
-- have been refused in silence. `code_hook_id` is the message's subject, the fifteenth the
-- one-subject check counts; the workspace's and the hub's administrators are told.
--
-- Safe for a rolling update: a nullable column, a NOT VALID foreign key and check, validated with
-- the index in 0354 outside a transaction. An older binary never writes the column.

-- +goose Up

ALTER TABLE notification ADD COLUMN IF NOT EXISTS code_hook_id uuid;
ALTER TABLE notification ADD CONSTRAINT notification_code_hook_id_fkey
  FOREIGN KEY (tenant_id, code_hook_id) REFERENCES code_hook (tenant_id, id) ON DELETE CASCADE NOT VALID;

ALTER TABLE notification DROP CONSTRAINT notification_one_subject_check;
ALTER TABLE notification ADD CONSTRAINT notification_one_subject_check
  CHECK (num_nonnulls(item_id, rule_id, subscription_id, private_hub_access_id, membership_id, page_id, agent_id,
    whiteboard_id, approval_id, baseline_id, mailbox_id, identity_provider_id, calendar_push_id,
    cmdb_connector_id, code_hook_id) <= 1)
  NOT VALID;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
