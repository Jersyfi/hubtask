-- A notification category for attestation rounds (UC-AST-10): a writer reminding the owners of
-- objects a campaign still waits for. The closed set of `notification.category` and
-- `notification_preference.category` takes ATTESTATION, and `cmdb_campaign_id` is the message's
-- subject, the sixteenth the one-subject check counts.
--
-- Safe for a rolling update: only widenings and a nullable column (rule 12). An older binary never
-- writes the new value or the column. The checks and the foreign key are NOT VALID here and
-- validated with the index in 0433 outside a transaction.

-- +goose Up

ALTER TABLE notification DROP CONSTRAINT notification_category_check;
ALTER TABLE notification ADD CONSTRAINT notification_category_check
  CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER','INTEGRATION','RETENTION','EMERGENCY_ACCESS','REQUEST_REPLY','SLA','POST','ATTESTATION')) NOT VALID;

ALTER TABLE notification_preference DROP CONSTRAINT notification_preference_category_check;
ALTER TABLE notification_preference ADD CONSTRAINT notification_preference_category_check
  CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER','INTEGRATION','RETENTION','EMERGENCY_ACCESS','REQUEST_REPLY','SLA','POST','ATTESTATION')) NOT VALID;

ALTER TABLE notification ADD COLUMN IF NOT EXISTS cmdb_campaign_id uuid;
ALTER TABLE notification ADD CONSTRAINT notification_cmdb_campaign_id_fkey
  FOREIGN KEY (tenant_id, cmdb_campaign_id) REFERENCES cmdb_campaign (tenant_id, id) ON DELETE CASCADE NOT VALID;

ALTER TABLE notification DROP CONSTRAINT notification_one_subject_check;
ALTER TABLE notification ADD CONSTRAINT notification_one_subject_check
  CHECK (num_nonnulls(item_id, rule_id, subscription_id, private_hub_access_id, membership_id, page_id, agent_id,
    whiteboard_id, approval_id, baseline_id, mailbox_id, identity_provider_id, calendar_push_id,
    cmdb_connector_id, code_hook_id, cmdb_campaign_id) <= 1)
  NOT VALID;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
