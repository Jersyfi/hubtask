-- The second half of what migrations 0211 to 0316 change on tables the last release (v0.9.1)
-- already holds data in (versioning-release.md §4): the indexes they need, built CONCURRENTLY, and
-- the constraints they added NOT VALID, validated. Without a transaction, so each statement takes
-- only locks writes pass (SHARE UPDATE EXCLUSIVE); IF NOT EXISTS for a retry after an interruption.
--
-- An interrupted concurrent build leaves an invalid index that IF NOT EXISTS would keep, and an
-- invalid unique index backs no key: the first block drops such a leftover of this file's indexes
-- (a short exclusive lock, bounded by lock_timeout) so the retry builds it again.
--
-- The push's reference to its feed (0278) needs the unique (tenant_id, id) on `calendar_feed`, so
-- it is added here, after that index - only where it is missing, as a database migrated before
-- this file was split has it already.

-- +goose NO TRANSACTION

-- +goose Up

-- +goose StatementBegin
DO $leftovers$
DECLARE
  leftover record;
BEGIN
  SET LOCAL lock_timeout = '5s';
  FOR leftover IN
    SELECT c.relname FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
     WHERE NOT i.indisvalid AND c.relname IN (
       'notification_page_idx', 'notification_agent_idx', 'notification_whiteboard_idx',
       'notification_approval_idx', 'notification_baseline_once', 'account_external_id_uq',
       'account_group_external_id_uq', 'calendar_feed_tenant_id_uq', 'saved_view_key_uq',
       'automation_rule_key_uq')
  LOOP
    EXECUTE format('DROP INDEX IF EXISTS %I', leftover.relname);
  END LOOP;
END
$leftovers$;
-- +goose StatementEnd

-- The cascades a purged page, agent, board or approval sets off read these (0232, 0253, 0266, 0269).
CREATE INDEX CONCURRENTLY IF NOT EXISTS notification_page_idx
  ON notification (tenant_id, page_id) WHERE page_id IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS notification_agent_idx
  ON notification (tenant_id, agent_id) WHERE agent_id IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS notification_whiteboard_idx
  ON notification (tenant_id, whiteboard_id) WHERE whiteboard_id IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS notification_approval_idx
  ON notification (tenant_id, approval_id) WHERE approval_id IS NOT NULL;
-- One exception notice per baseline and recipient (0273).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS notification_baseline_once
  ON notification (tenant_id, baseline_id, recipient_id, channel) WHERE baseline_id IS NOT NULL;
-- The identifier a directory knows a person or a group by (0256).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS account_external_id_uq
  ON account (tenant_id, external_id) WHERE external_id IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS account_group_external_id_uq
  ON account_group (tenant_id, external_id) WHERE external_id IS NOT NULL;
-- A view's and a rule's stable key where the block lives (0286).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS saved_view_key_uq ON saved_view
  (tenant_id, scope_type, coalesce(scope_id, '00000000-0000-0000-0000-000000000000'::uuid), key)
  WHERE key IS NOT NULL;
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS automation_rule_key_uq ON automation_rule
  (tenant_id, scope_type, coalesce(scope_id, '00000000-0000-0000-0000-000000000000'::uuid), key)
  WHERE key IS NOT NULL AND deleted_at IS NULL;
-- A feed's identity per workspace, which the push's tenant-scoped reference needs (0278).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS calendar_feed_tenant_id_uq ON calendar_feed (tenant_id, id);

-- +goose StatementBegin
DO $feed_reference$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'calendar_push_feed_fkey'
                   AND conrelid = 'calendar_push'::regclass) THEN
    SET LOCAL lock_timeout = '5s';
    ALTER TABLE calendar_push ADD CONSTRAINT calendar_push_feed_fkey FOREIGN KEY (tenant_id, feed_id)
      REFERENCES calendar_feed (tenant_id, id) ON DELETE CASCADE NOT VALID;
  END IF;
END
$feed_reference$;
-- +goose StatementEnd
ALTER TABLE calendar_push VALIDATE CONSTRAINT calendar_push_feed_fkey;

-- The constraints as the last migration to touch each left them.
ALTER TABLE custom_field_definition VALIDATE CONSTRAINT custom_field_definition_kind_check;
ALTER TABLE custom_field_definition VALIDATE CONSTRAINT custom_field_definition_currency_check;
ALTER TABLE custom_field_definition VALIDATE CONSTRAINT custom_field_definition_formula_check;
ALTER TABLE custom_field_definition VALIDATE CONSTRAINT cfd_read_restricted_ck;
ALTER TABLE rule_run VALIDATE CONSTRAINT rule_run_trigger_check;
ALTER TABLE notification VALIDATE CONSTRAINT notification_page_id_fkey;
ALTER TABLE notification VALIDATE CONSTRAINT notification_agent_id_fkey;
ALTER TABLE notification VALIDATE CONSTRAINT notification_whiteboard_id_fkey;
ALTER TABLE notification VALIDATE CONSTRAINT notification_approval_id_fkey;
ALTER TABLE notification VALIDATE CONSTRAINT notification_baseline_id_fkey;
ALTER TABLE notification VALIDATE CONSTRAINT notification_one_subject_check;
ALTER TABLE notification VALIDATE CONSTRAINT notification_category_check;
ALTER TABLE notification_preference VALIDATE CONSTRAINT notification_preference_category_check;
ALTER TABLE ai_suggestion VALIDATE CONSTRAINT ai_suggestion_agent_id_fkey;
ALTER TABLE import_run VALIDATE CONSTRAINT import_run_kind_check;
ALTER TABLE account VALIDATE CONSTRAINT account_external_id_length_check;
ALTER TABLE account_group VALIDATE CONSTRAINT account_group_external_id_length_check;
ALTER TABLE webhook_subscription VALIDATE CONSTRAINT webhook_subscription_format_check;
ALTER TABLE session VALIDATE CONSTRAINT session_viewing_as_fkey;
ALTER TABLE session VALIDATE CONSTRAINT session_viewing_as_check;
ALTER TABLE identity_provider VALIDATE CONSTRAINT identity_provider_kind_check;
ALTER TABLE identity_provider VALIDATE CONSTRAINT identity_provider_saml_whole;
ALTER TABLE media_object VALIDATE CONSTRAINT media_object_status_check;
ALTER TABLE media_object VALIDATE CONSTRAINT media_object_usage_check;
ALTER TABLE mailbox VALIDATE CONSTRAINT mailbox_protocol_check;
ALTER TABLE automation_rule VALIDATE CONSTRAINT automation_rule_scope_type_check;
ALTER TABLE automation_rule VALIDATE CONSTRAINT automation_rule_person_run_as_check;
ALTER TABLE container VALIDATE CONSTRAINT container_logo_media_fkey;

-- Two that earlier files left NOT VALID for good, which a restore drill counts as a promise the data
-- may break (constraints_validated): the embeddings' model bound (0129), only where pgvector made
-- the table, and the address beside its text (0298).
ALTER TABLE entry_field_value VALIDATE CONSTRAINT entry_field_value_addr_check;
-- +goose StatementBegin
DO $embedding_bound$
BEGIN
  IF to_regclass('item_embedding') IS NOT NULL THEN
    ALTER TABLE item_embedding VALIDATE CONSTRAINT item_embedding_model_check;
  END IF;
END
$embedding_bound$;
-- +goose StatementEnd

-- +goose Down

DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
