-- A notification about a connected mailbox: that it has refused for over an hour, told to the
-- workspace's administrators once when it turns failing (ADR-0094). `mailbox_id` is the message's
-- subject, the eleventh the one-subject check counts.
--
-- Safe for a rolling update: a nullable column, a NOT VALID foreign key and check, validated with
-- the index in 0339 outside a transaction. An older binary never writes the column.

-- +goose Up

ALTER TABLE notification ADD COLUMN IF NOT EXISTS mailbox_id uuid;
ALTER TABLE notification ADD CONSTRAINT notification_mailbox_id_fkey
  FOREIGN KEY (tenant_id, mailbox_id) REFERENCES mailbox (tenant_id, id) ON DELETE CASCADE NOT VALID;

ALTER TABLE notification DROP CONSTRAINT notification_one_subject_check;
ALTER TABLE notification ADD CONSTRAINT notification_one_subject_check
  CHECK (num_nonnulls(item_id, rule_id, subscription_id, private_hub_access_id, membership_id, page_id, agent_id,
    whiteboard_id, approval_id, baseline_id, mailbox_id) <= 1)
  NOT VALID;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
