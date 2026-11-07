-- A notification about a connection that stopped for good: a calendar push the provider refused
-- (the grant revoked, the feed gone), told to its owner, and a CMDB connector that switched itself
-- off after its failures, told to the workspace's and the hub's administrators. Each stopped
-- silently before; `calendar_push_id` and `cmdb_connector_id` are the message's subject, the
-- thirteenth and fourteenth the one-subject check counts.
--
-- Safe for a rolling update: nullable columns, NOT VALID foreign keys and check, validated with
-- the indexes in 0350 outside a transaction. An older binary never writes the columns.

-- +goose Up

ALTER TABLE notification ADD COLUMN IF NOT EXISTS calendar_push_id uuid;
ALTER TABLE notification ADD COLUMN IF NOT EXISTS cmdb_connector_id uuid;
ALTER TABLE notification ADD CONSTRAINT notification_calendar_push_id_fkey
  FOREIGN KEY (tenant_id, calendar_push_id) REFERENCES calendar_push (tenant_id, id) ON DELETE CASCADE NOT VALID;
ALTER TABLE notification ADD CONSTRAINT notification_cmdb_connector_id_fkey
  FOREIGN KEY (tenant_id, cmdb_connector_id) REFERENCES cmdb_connector (tenant_id, id) ON DELETE CASCADE NOT VALID;

ALTER TABLE notification DROP CONSTRAINT notification_one_subject_check;
ALTER TABLE notification ADD CONSTRAINT notification_one_subject_check
  CHECK (num_nonnulls(item_id, rule_id, subscription_id, private_hub_access_id, membership_id, page_id, agent_id,
    whiteboard_id, approval_id, baseline_id, mailbox_id, identity_provider_id, calendar_push_id,
    cmdb_connector_id) <= 1)
  NOT VALID;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
