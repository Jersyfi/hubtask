-- Tenant-scoped foreign keys, step 3 of 3: validating them (ADR-0024).
--
-- Step 2 added the constraints NOT VALID: they hold for every new row and say nothing about the
-- rows that were already there. This is the scan that checks those, in a step of its own so that it
-- can take its time without the deploy waiting on it.
--
-- An installation whose data already violates a constraint fails here rather than at deploy time,
-- and the constraint name in the error says which reference is wrong. The offending rows are then
-- one query away - for the collection of an item, for example:
--
--   SELECT i.id, i.tenant_id, i.collection_id FROM work_item i
--   LEFT JOIN container c ON c.tenant_id = i.tenant_id AND c.id = i.collection_id
--   WHERE i.collection_id IS NOT NULL AND c.id IS NULL;
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003):
-- VALIDATE CONSTRAINT takes a SHARE UPDATE EXCLUSIVE lock, which blocks neither reads nor writes.
-- Outside a transaction, so each constraint takes and releases its own lock rather than one lock
-- being held for all thirty.

-- +goose NO TRANSACTION

-- +goose Up

ALTER TABLE access_token VALIDATE CONSTRAINT access_token_account_id_fkey;
ALTER TABLE account_group_member VALIDATE CONSTRAINT account_group_member_account_id_fkey;
ALTER TABLE account_group_member VALIDATE CONSTRAINT account_group_member_group_id_fkey;
ALTER TABLE automation_rule VALIDATE CONSTRAINT automation_rule_run_as_fkey;
ALTER TABLE bucket VALIDATE CONSTRAINT bucket_collection_id_fkey;
ALTER TABLE calendar_feed VALIDATE CONSTRAINT calendar_feed_account_id_fkey;
ALTER TABLE calendar_feed VALIDATE CONSTRAINT calendar_feed_view_id_fkey;
ALTER TABLE comment VALIDATE CONSTRAINT comment_item_id_fkey;
ALTER TABLE comment VALIDATE CONSTRAINT comment_parent_comment_id_fkey;
ALTER TABLE consent_record VALIDATE CONSTRAINT consent_record_account_id_fkey;
ALTER TABLE container VALIDATE CONSTRAINT container_parent_id_fkey;
ALTER TABLE custom_field_definition VALIDATE CONSTRAINT custom_field_definition_collection_id_fkey;
ALTER TABLE item_attachment VALIDATE CONSTRAINT item_attachment_item_id_fkey;
ALTER TABLE item_attachment VALIDATE CONSTRAINT item_attachment_media_id_fkey;
ALTER TABLE item_label VALIDATE CONSTRAINT item_label_item_id_fkey;
ALTER TABLE item_label VALIDATE CONSTRAINT item_label_label_id_fkey;
ALTER TABLE item_member VALIDATE CONSTRAINT item_member_account_id_fkey;
ALTER TABLE item_member VALIDATE CONSTRAINT item_member_item_id_fkey;
ALTER TABLE label VALIDATE CONSTRAINT label_collection_id_fkey;
ALTER TABLE membership VALIDATE CONSTRAINT membership_account_id_fkey;
ALTER TABLE membership VALIDATE CONSTRAINT membership_group_id_fkey;
ALTER TABLE recurrence_rule VALIDATE CONSTRAINT recurrence_rule_source_item_id_fkey;
ALTER TABLE reminder VALIDATE CONSTRAINT reminder_item_id_fkey;
ALTER TABLE rule_run VALIDATE CONSTRAINT rule_run_rule_id_fkey;
ALTER TABLE sync_device VALIDATE CONSTRAINT sync_device_account_id_fkey;
ALTER TABLE webhook_delivery VALIDATE CONSTRAINT webhook_delivery_subscription_id_fkey;
ALTER TABLE work_item VALIDATE CONSTRAINT work_item_assignee_id_fkey;
ALTER TABLE work_item VALIDATE CONSTRAINT work_item_bucket_id_fkey;
ALTER TABLE work_item VALIDATE CONSTRAINT work_item_collection_id_fkey;
ALTER TABLE work_item VALIDATE CONSTRAINT work_item_parent_id_fkey;

-- +goose Down

-- Nothing to undo: a validated constraint is the same constraint.
SELECT 1;
