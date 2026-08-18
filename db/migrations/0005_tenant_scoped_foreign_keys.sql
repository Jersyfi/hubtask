-- Tenant-scoped foreign keys, step 2 of 3: the composite constraints (ADR-0024).
--
-- Each single-column foreign key is replaced by one that carries the tenant. PostgreSQL checks
-- referential integrity in triggers that run as the table owner, which row level security does not
-- reach - so without the tenant in the key, a row in one tenant can reference, and a cascade can
-- delete, a row in another.
--
-- Two details are not cosmetic. `MATCH SIMPLE` is the default and is what keeps an optional
-- reference optional: a row whose reference is NULL is not checked at all. And `ON DELETE SET NULL`
-- carries a column list, because the naive form would null `tenant_id` too - which is NOT NULL, so
-- the delete would fail. That form is accepted when it is declared and fails only when it fires,
-- which is why the gate checks the delete rule and not only the columns.
--
-- The backup family is deliberately absent. Its `tenant_id` is nullable because NULL means
-- installation-wide, and a composite key there would forbid a tenant using an installation-wide
-- target and switch the check off for the rows that keep a NULL tenant.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003):
-- every constraint is added NOT VALID, so existing rows are not scanned and no long lock is taken.
-- Old application code keeps working throughout - it already writes references only inside its own
-- tenant, which is what makes the constraint addable at all. Step 3 validates.

-- +goose Up

ALTER TABLE access_token
  DROP CONSTRAINT access_token_account_id_fkey,
  ADD CONSTRAINT access_token_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE account_group_member
  DROP CONSTRAINT account_group_member_account_id_fkey,
  ADD CONSTRAINT account_group_member_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE account_group_member
  DROP CONSTRAINT account_group_member_group_id_fkey,
  ADD CONSTRAINT account_group_member_group_id_fkey
    FOREIGN KEY (tenant_id, group_id) REFERENCES account_group (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE automation_rule
  DROP CONSTRAINT automation_rule_run_as_fkey,
  ADD CONSTRAINT automation_rule_run_as_fkey
    FOREIGN KEY (tenant_id, run_as) REFERENCES account (tenant_id, id) ON DELETE RESTRICT NOT VALID;

ALTER TABLE bucket
  DROP CONSTRAINT bucket_collection_id_fkey,
  ADD CONSTRAINT bucket_collection_id_fkey
    FOREIGN KEY (tenant_id, collection_id) REFERENCES container (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE calendar_feed
  DROP CONSTRAINT calendar_feed_account_id_fkey,
  ADD CONSTRAINT calendar_feed_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE calendar_feed
  DROP CONSTRAINT calendar_feed_view_id_fkey,
  ADD CONSTRAINT calendar_feed_view_id_fkey
    FOREIGN KEY (tenant_id, view_id) REFERENCES saved_view (tenant_id, id)
      ON DELETE SET NULL (view_id) NOT VALID;

ALTER TABLE comment
  DROP CONSTRAINT comment_item_id_fkey,
  ADD CONSTRAINT comment_item_id_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE comment
  DROP CONSTRAINT comment_parent_comment_id_fkey,
  ADD CONSTRAINT comment_parent_comment_id_fkey
    FOREIGN KEY (tenant_id, parent_comment_id) REFERENCES comment (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE consent_record
  DROP CONSTRAINT consent_record_account_id_fkey,
  ADD CONSTRAINT consent_record_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE container
  DROP CONSTRAINT container_parent_id_fkey,
  ADD CONSTRAINT container_parent_id_fkey
    FOREIGN KEY (tenant_id, parent_id) REFERENCES container (tenant_id, id)
      ON DELETE RESTRICT NOT VALID;

ALTER TABLE custom_field_definition
  DROP CONSTRAINT custom_field_definition_collection_id_fkey,
  ADD CONSTRAINT custom_field_definition_collection_id_fkey
    FOREIGN KEY (tenant_id, collection_id) REFERENCES container (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE item_attachment
  DROP CONSTRAINT item_attachment_item_id_fkey,
  ADD CONSTRAINT item_attachment_item_id_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE item_attachment
  DROP CONSTRAINT item_attachment_media_id_fkey,
  ADD CONSTRAINT item_attachment_media_id_fkey
    FOREIGN KEY (tenant_id, media_id) REFERENCES media_object (tenant_id, id)
      ON DELETE RESTRICT NOT VALID;

ALTER TABLE item_label
  DROP CONSTRAINT item_label_item_id_fkey,
  ADD CONSTRAINT item_label_item_id_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE item_label
  DROP CONSTRAINT item_label_label_id_fkey,
  ADD CONSTRAINT item_label_label_id_fkey
    FOREIGN KEY (tenant_id, label_id) REFERENCES label (tenant_id, id) ON DELETE CASCADE NOT VALID;

ALTER TABLE item_member
  DROP CONSTRAINT item_member_account_id_fkey,
  ADD CONSTRAINT item_member_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE item_member
  DROP CONSTRAINT item_member_item_id_fkey,
  ADD CONSTRAINT item_member_item_id_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE label
  DROP CONSTRAINT label_collection_id_fkey,
  ADD CONSTRAINT label_collection_id_fkey
    FOREIGN KEY (tenant_id, collection_id) REFERENCES container (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE membership
  DROP CONSTRAINT membership_account_id_fkey,
  ADD CONSTRAINT membership_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE membership
  DROP CONSTRAINT membership_group_id_fkey,
  ADD CONSTRAINT membership_group_id_fkey
    FOREIGN KEY (tenant_id, group_id) REFERENCES account_group (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE recurrence_rule
  DROP CONSTRAINT recurrence_rule_source_item_id_fkey,
  ADD CONSTRAINT recurrence_rule_source_item_id_fkey
    FOREIGN KEY (tenant_id, source_item_id) REFERENCES work_item (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE reminder
  DROP CONSTRAINT reminder_item_id_fkey,
  ADD CONSTRAINT reminder_item_id_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE rule_run
  DROP CONSTRAINT rule_run_rule_id_fkey,
  ADD CONSTRAINT rule_run_rule_id_fkey
    FOREIGN KEY (tenant_id, rule_id) REFERENCES automation_rule (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE sync_device
  DROP CONSTRAINT sync_device_account_id_fkey,
  ADD CONSTRAINT sync_device_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE webhook_delivery
  DROP CONSTRAINT webhook_delivery_subscription_id_fkey,
  ADD CONSTRAINT webhook_delivery_subscription_id_fkey
    FOREIGN KEY (tenant_id, subscription_id) REFERENCES webhook_subscription (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE work_item
  DROP CONSTRAINT work_item_assignee_id_fkey,
  ADD CONSTRAINT work_item_assignee_id_fkey
    FOREIGN KEY (tenant_id, assignee_id) REFERENCES account (tenant_id, id)
      ON DELETE SET NULL (assignee_id) NOT VALID;

ALTER TABLE work_item
  DROP CONSTRAINT work_item_bucket_id_fkey,
  ADD CONSTRAINT work_item_bucket_id_fkey
    FOREIGN KEY (tenant_id, bucket_id) REFERENCES bucket (tenant_id, id)
      ON DELETE SET NULL (bucket_id) NOT VALID;

ALTER TABLE work_item
  DROP CONSTRAINT work_item_collection_id_fkey,
  ADD CONSTRAINT work_item_collection_id_fkey
    FOREIGN KEY (tenant_id, collection_id) REFERENCES container (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE work_item
  DROP CONSTRAINT work_item_parent_id_fkey,
  ADD CONSTRAINT work_item_parent_id_fkey
    FOREIGN KEY (tenant_id, parent_id) REFERENCES work_item (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

-- +goose Down

-- Going back would mean re-creating the single-column keys and thereby the cross-tenant hole
-- ADR-0024 measured. The answer to a bad deploy is a restore, not a schema that has moved back.
SELECT 1;
