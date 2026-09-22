-- A notification can be about something that is not an entry (issue 814): the rule the check or
-- the failure streak switched off, the webhook subscription the engine stopped calling. Both were
-- written into item_id, whose foreign key points at work_item, so the insert failed - and with it
-- the check's whole transaction, which is why the automation list answered 503 whenever a rule
-- was broken. Each subject gets a column of its own with its own key; at most one is set.
--
-- Expand only: two nullable columns and a constraint every existing row satisfies, because no
-- row with a rule or a subscription in item_id ever committed.

-- +goose Up
ALTER TABLE notification
  ADD COLUMN rule_id uuid,
  ADD COLUMN subscription_id uuid,
  ADD CONSTRAINT notification_rule_id_fkey
    FOREIGN KEY (tenant_id, rule_id) REFERENCES automation_rule (tenant_id, id) ON DELETE CASCADE,
  ADD CONSTRAINT notification_subscription_id_fkey
    FOREIGN KEY (tenant_id, subscription_id) REFERENCES webhook_subscription (tenant_id, id) ON DELETE CASCADE,
  ADD CONSTRAINT notification_one_subject_check
    CHECK (num_nonnulls(item_id, rule_id, subscription_id) <= 1);

-- +goose Down
ALTER TABLE notification
  DROP CONSTRAINT notification_one_subject_check,
  DROP CONSTRAINT notification_subscription_id_fkey,
  DROP CONSTRAINT notification_rule_id_fkey,
  DROP COLUMN subscription_id,
  DROP COLUMN rule_id;
