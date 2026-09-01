-- The statements a restore writes a tenant through (E-06, backup-restore.md §8).
--
-- One trio per entity: does the tenant already hold this row, write it, and - for REPLACE_TENANT -
-- empty the table. They are as alike as the schema lets them be, and they are hand-written for the
-- reason the export's are: what differs between them is a decision per entity - which columns
-- identify a row, and which of them a generated column may not be written into - and a generator
-- would have to be told all of that anyway.
--
-- **The row arrives as one jsonb value and is unpacked by the database.** `jsonb_populate_record`
-- against the table's own row type is what makes the import a statement rather than a string with
-- thirty-eight parameters in it, and rule 9 is the reason that matters: no byte of an archive ever
-- becomes SQL text. A field the archive carries and the table does not have is dropped by the
-- unpacking; a column the archive does not carry arrives NULL.
--
-- `tenant_id` is not taken from the archive. It is `current_tenant_id()`, the same value row level
-- security compares against, so a restore cannot write into another tenant even deliberately -
-- which is BK-10 at the layer where it cannot be forgotten (ADR-0010).
--
-- The upsert's `WHERE @overwrite` is the conflict rule, in the statement rather than in a branch
-- above it: SKIP is the same statement with the flag false, and a row that was neither inserted nor
-- updated returns nothing at all, which is how the caller tells the three outcomes apart.

-- name: ImportTenant :execrows
INSERT INTO tenant (
  id,
  slug,
  display_name,
  status,
  default_locale,
  default_time_zone,
  settings,
  created_at,
  updated_at,
  deleted_at,
  version
)
SELECT
  r.id,
  r.slug,
  r.display_name,
  r.status,
  r.default_locale,
  r.default_time_zone,
  r.settings,
  r.created_at,
  r.updated_at,
  r.deleted_at,
  r.version
FROM jsonb_populate_record(
  NULL::tenant,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  slug = EXCLUDED.slug,
  display_name = EXCLUDED.display_name,
  status = EXCLUDED.status,
  default_locale = EXCLUDED.default_locale,
  default_time_zone = EXCLUDED.default_time_zone,
  settings = EXCLUDED.settings,
  created_at = EXCLUDED.created_at,
  updated_at = EXCLUDED.updated_at,
  deleted_at = EXCLUDED.deleted_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsTenant :one
SELECT EXISTS (SELECT 1 FROM tenant WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ImportAccount :execrows
INSERT INTO account (
  id,
  tenant_id,
  kind,
  email,
  display_name,
  external_subject,
  password_hash,
  locale,
  time_zone,
  week_start,
  status,
  ai_consent,
  created_at,
  updated_at,
  deleted_at,
  version
)
SELECT
  r.id,
  r.tenant_id,
  r.kind,
  r.email,
  r.display_name,
  r.external_subject,
  r.password_hash,
  r.locale,
  r.time_zone,
  r.week_start,
  r.status,
  r.ai_consent,
  r.created_at,
  r.updated_at,
  r.deleted_at,
  r.version
FROM jsonb_populate_record(
  NULL::account,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  kind = EXCLUDED.kind,
  email = EXCLUDED.email,
  display_name = EXCLUDED.display_name,
  external_subject = EXCLUDED.external_subject,
  password_hash = EXCLUDED.password_hash,
  locale = EXCLUDED.locale,
  time_zone = EXCLUDED.time_zone,
  week_start = EXCLUDED.week_start,
  status = EXCLUDED.status,
  ai_consent = EXCLUDED.ai_consent,
  created_at = EXCLUDED.created_at,
  updated_at = EXCLUDED.updated_at,
  deleted_at = EXCLUDED.deleted_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsAccount :one
SELECT EXISTS (SELECT 1 FROM account WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearAccount :execrows
DELETE FROM account;

-- name: ImportAccountGroup :execrows
INSERT INTO account_group (
  id,
  tenant_id,
  name,
  description,
  created_at,
  version
)
SELECT
  r.id,
  r.tenant_id,
  r.name,
  r.description,
  r.created_at,
  r.version
FROM jsonb_populate_record(
  NULL::account_group,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  name = EXCLUDED.name,
  description = EXCLUDED.description,
  created_at = EXCLUDED.created_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsAccountGroup :one
SELECT EXISTS (SELECT 1 FROM account_group WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearAccountGroup :execrows
DELETE FROM account_group;

-- name: ImportAccountGroupMember :execrows
INSERT INTO account_group_member (
  tenant_id,
  group_id,
  account_id
)
SELECT
  r.tenant_id,
  r.group_id,
  r.account_id
FROM jsonb_populate_record(
  NULL::account_group_member,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (group_id, account_id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsAccountGroupMember :one
SELECT EXISTS (SELECT 1 FROM account_group_member WHERE group_id = (sqlc.arg('payload')::jsonb->>'group_id')::uuid AND account_id = (sqlc.arg('payload')::jsonb->>'account_id')::uuid) AS held;

-- name: ClearAccountGroupMember :execrows
DELETE FROM account_group_member;

-- name: ImportMembership :execrows
INSERT INTO membership (
  id,
  tenant_id,
  account_id,
  group_id,
  scope_type,
  scope_id,
  role,
  created_at
)
SELECT
  r.id,
  r.tenant_id,
  r.account_id,
  r.group_id,
  r.scope_type,
  r.scope_id,
  r.role,
  r.created_at
FROM jsonb_populate_record(
  NULL::membership,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  account_id = EXCLUDED.account_id,
  group_id = EXCLUDED.group_id,
  scope_type = EXCLUDED.scope_type,
  scope_id = EXCLUDED.scope_id,
  role = EXCLUDED.role,
  created_at = EXCLUDED.created_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsMembership :one
SELECT EXISTS (SELECT 1 FROM membership WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearMembership :execrows
DELETE FROM membership;

-- name: ImportContainer :execrows
INSERT INTO container (
  id,
  tenant_id,
  type,
  parent_id,
  name,
  description,
  icon,
  color_token,
  order_key,
  policies,
  archived_at,
  deleted_at,
  trash_batch_id,
  created_by,
  created_at,
  updated_at,
  version
)
SELECT
  r.id,
  r.tenant_id,
  r.type,
  r.parent_id,
  r.name,
  r.description,
  r.icon,
  r.color_token,
  r.order_key,
  r.policies,
  r.archived_at,
  r.deleted_at,
  r.trash_batch_id,
  r.created_by,
  r.created_at,
  r.updated_at,
  r.version
FROM jsonb_populate_record(
  NULL::container,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  type = EXCLUDED.type,
  parent_id = EXCLUDED.parent_id,
  name = EXCLUDED.name,
  description = EXCLUDED.description,
  icon = EXCLUDED.icon,
  color_token = EXCLUDED.color_token,
  order_key = EXCLUDED.order_key,
  policies = EXCLUDED.policies,
  archived_at = EXCLUDED.archived_at,
  deleted_at = EXCLUDED.deleted_at,
  trash_batch_id = EXCLUDED.trash_batch_id,
  created_by = EXCLUDED.created_by,
  created_at = EXCLUDED.created_at,
  updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsContainer :one
SELECT EXISTS (SELECT 1 FROM container WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearContainer :execrows
DELETE FROM container;

-- name: ImportBucket :execrows
INSERT INTO bucket (
  id,
  tenant_id,
  collection_id,
  name,
  order_key,
  wip_limit,
  is_done_bucket,
  color_token,
  deleted_at,
  version
)
SELECT
  r.id,
  r.tenant_id,
  r.collection_id,
  r.name,
  r.order_key,
  r.wip_limit,
  r.is_done_bucket,
  r.color_token,
  r.deleted_at,
  r.version
FROM jsonb_populate_record(
  NULL::bucket,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  collection_id = EXCLUDED.collection_id,
  name = EXCLUDED.name,
  order_key = EXCLUDED.order_key,
  wip_limit = EXCLUDED.wip_limit,
  is_done_bucket = EXCLUDED.is_done_bucket,
  color_token = EXCLUDED.color_token,
  deleted_at = EXCLUDED.deleted_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsBucket :one
SELECT EXISTS (SELECT 1 FROM bucket WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearBucket :execrows
DELETE FROM bucket;

-- name: ImportLabel :execrows
INSERT INTO label (
  id,
  tenant_id,
  collection_id,
  name,
  color_token,
  description,
  deleted_at,
  version
)
SELECT
  r.id,
  r.tenant_id,
  r.collection_id,
  r.name,
  r.color_token,
  r.description,
  r.deleted_at,
  r.version
FROM jsonb_populate_record(
  NULL::label,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  collection_id = EXCLUDED.collection_id,
  name = EXCLUDED.name,
  color_token = EXCLUDED.color_token,
  description = EXCLUDED.description,
  deleted_at = EXCLUDED.deleted_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsLabel :one
SELECT EXISTS (SELECT 1 FROM label WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearLabel :execrows
DELETE FROM label;

-- name: ImportCustomFieldDefinition :execrows
INSERT INTO custom_field_definition (
  id,
  tenant_id,
  collection_id,
  key,
  kind,
  options,
  is_required,
  applies_to,
  deleted_at,
  created_at,
  updated_at,
  version
)
SELECT
  r.id,
  r.tenant_id,
  r.collection_id,
  r.key,
  r.kind,
  r.options,
  r.is_required,
  r.applies_to,
  r.deleted_at,
  r.created_at,
  r.updated_at,
  r.version
FROM jsonb_populate_record(
  NULL::custom_field_definition,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  collection_id = EXCLUDED.collection_id,
  key = EXCLUDED.key,
  kind = EXCLUDED.kind,
  options = EXCLUDED.options,
  is_required = EXCLUDED.is_required,
  applies_to = EXCLUDED.applies_to,
  deleted_at = EXCLUDED.deleted_at,
  created_at = EXCLUDED.created_at,
  updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCustomFieldDefinition :one
SELECT EXISTS (SELECT 1 FROM custom_field_definition WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCustomFieldDefinition :execrows
DELETE FROM custom_field_definition;

-- name: ImportWorkItem :execrows
-- `search_vector` is generated by the database and is therefore not written back: the archive carries
-- what the column held, and PostgreSQL refuses an insert into one.
INSERT INTO work_item (
  id,
  tenant_id,
  collection_id,
  type,
  parent_id,
  path,
  depth,
  title,
  notes,
  is_completed,
  completed_at,
  completed_by,
  bucket_id,
  order_key,
  start_at,
  due_at,
  due_date_only,
  due_time_zone,
  assignee_id,
  cover_kind,
  cover_color_token,
  cover_media_id,
  custom_fields,
  recurrence_rule_id,
  origin_jumble_id,
  content_language,
  archived_at,
  deleted_at,
  trash_batch_id,
  created_by,
  created_at,
  updated_at,
  version,
  custom_field_refs,
  search_document,
  due_soon_announced_at,
  overdue_announced_at,
  retention_pending_until,
  retention_rule_id,
  retention_action,
  retention_blocked_by
)
SELECT
  r.id,
  r.tenant_id,
  r.collection_id,
  r.type,
  r.parent_id,
  r.path,
  r.depth,
  r.title,
  r.notes,
  r.is_completed,
  r.completed_at,
  r.completed_by,
  r.bucket_id,
  r.order_key,
  r.start_at,
  r.due_at,
  r.due_date_only,
  r.due_time_zone,
  r.assignee_id,
  r.cover_kind,
  r.cover_color_token,
  r.cover_media_id,
  r.custom_fields,
  r.recurrence_rule_id,
  r.origin_jumble_id,
  r.content_language,
  r.archived_at,
  r.deleted_at,
  r.trash_batch_id,
  r.created_by,
  r.created_at,
  r.updated_at,
  r.version,
  r.custom_field_refs,
  r.search_document,
  r.due_soon_announced_at,
  r.overdue_announced_at,
  r.retention_pending_until,
  r.retention_rule_id,
  r.retention_action,
  r.retention_blocked_by
FROM jsonb_populate_record(
  NULL::work_item,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  collection_id = EXCLUDED.collection_id,
  type = EXCLUDED.type,
  parent_id = EXCLUDED.parent_id,
  path = EXCLUDED.path,
  depth = EXCLUDED.depth,
  title = EXCLUDED.title,
  notes = EXCLUDED.notes,
  is_completed = EXCLUDED.is_completed,
  completed_at = EXCLUDED.completed_at,
  completed_by = EXCLUDED.completed_by,
  bucket_id = EXCLUDED.bucket_id,
  order_key = EXCLUDED.order_key,
  start_at = EXCLUDED.start_at,
  due_at = EXCLUDED.due_at,
  due_date_only = EXCLUDED.due_date_only,
  due_time_zone = EXCLUDED.due_time_zone,
  assignee_id = EXCLUDED.assignee_id,
  cover_kind = EXCLUDED.cover_kind,
  cover_color_token = EXCLUDED.cover_color_token,
  cover_media_id = EXCLUDED.cover_media_id,
  custom_fields = EXCLUDED.custom_fields,
  recurrence_rule_id = EXCLUDED.recurrence_rule_id,
  origin_jumble_id = EXCLUDED.origin_jumble_id,
  content_language = EXCLUDED.content_language,
  archived_at = EXCLUDED.archived_at,
  deleted_at = EXCLUDED.deleted_at,
  trash_batch_id = EXCLUDED.trash_batch_id,
  created_by = EXCLUDED.created_by,
  created_at = EXCLUDED.created_at,
  updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version,
  custom_field_refs = EXCLUDED.custom_field_refs,
  search_document = EXCLUDED.search_document,
  due_soon_announced_at = EXCLUDED.due_soon_announced_at,
  overdue_announced_at = EXCLUDED.overdue_announced_at,
  retention_pending_until = EXCLUDED.retention_pending_until,
  retention_rule_id = EXCLUDED.retention_rule_id,
  retention_action = EXCLUDED.retention_action,
  retention_blocked_by = EXCLUDED.retention_blocked_by
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsWorkItem :one
SELECT EXISTS (SELECT 1 FROM work_item WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearWorkItem :execrows
DELETE FROM work_item;

-- name: ImportItemLabel :execrows
INSERT INTO item_label (
  tenant_id,
  item_id,
  label_id
)
SELECT
  r.tenant_id,
  r.item_id,
  r.label_id
FROM jsonb_populate_record(
  NULL::item_label,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (item_id, label_id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsItemLabel :one
SELECT EXISTS (SELECT 1 FROM item_label WHERE item_id = (sqlc.arg('payload')::jsonb->>'item_id')::uuid AND label_id = (sqlc.arg('payload')::jsonb->>'label_id')::uuid) AS held;

-- name: ClearItemLabel :execrows
DELETE FROM item_label;

-- name: ImportItemMember :execrows
INSERT INTO item_member (
  tenant_id,
  item_id,
  account_id
)
SELECT
  r.tenant_id,
  r.item_id,
  r.account_id
FROM jsonb_populate_record(
  NULL::item_member,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (item_id, account_id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsItemMember :one
SELECT EXISTS (SELECT 1 FROM item_member WHERE item_id = (sqlc.arg('payload')::jsonb->>'item_id')::uuid AND account_id = (sqlc.arg('payload')::jsonb->>'account_id')::uuid) AS held;

-- name: ClearItemMember :execrows
DELETE FROM item_member;

-- name: ImportComment :execrows
INSERT INTO comment (
  id,
  tenant_id,
  item_id,
  author_id,
  parent_comment_id,
  body,
  created_at,
  edited_at,
  deleted_at,
  version
)
SELECT
  r.id,
  r.tenant_id,
  r.item_id,
  r.author_id,
  r.parent_comment_id,
  r.body,
  r.created_at,
  r.edited_at,
  r.deleted_at,
  r.version
FROM jsonb_populate_record(
  NULL::comment,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  item_id = EXCLUDED.item_id,
  author_id = EXCLUDED.author_id,
  parent_comment_id = EXCLUDED.parent_comment_id,
  body = EXCLUDED.body,
  created_at = EXCLUDED.created_at,
  edited_at = EXCLUDED.edited_at,
  deleted_at = EXCLUDED.deleted_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsComment :one
SELECT EXISTS (SELECT 1 FROM comment WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearComment :execrows
DELETE FROM comment;

-- name: ImportActivityEntry :execrows
INSERT INTO activity_entry (
  id,
  tenant_id,
  item_id,
  container_id,
  actor_type,
  actor_id,
  verb,
  change_set,
  occurred_at,
  correlation_id,
  causation_id
)
SELECT
  r.id,
  r.tenant_id,
  r.item_id,
  r.container_id,
  r.actor_type,
  r.actor_id,
  r.verb,
  r.change_set,
  r.occurred_at,
  r.correlation_id,
  r.causation_id
FROM jsonb_populate_record(
  NULL::activity_entry,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
-- The conflict target is the partitioned key since H-09: a partitioned table cannot hold a
-- unique index on id alone, and occurred_at is immutable on an activity entry, so the pair
-- names the same row the old target did. tenant_id and occurred_at leave the update list -
-- they are the key now.
ON CONFLICT (tenant_id, occurred_at, id) DO UPDATE SET
  item_id = EXCLUDED.item_id,
  container_id = EXCLUDED.container_id,
  actor_type = EXCLUDED.actor_type,
  actor_id = EXCLUDED.actor_id,
  verb = EXCLUDED.verb,
  change_set = EXCLUDED.change_set,
  correlation_id = EXCLUDED.correlation_id,
  causation_id = EXCLUDED.causation_id
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsActivityEntry :one
SELECT EXISTS (SELECT 1 FROM activity_entry WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearActivityEntry :execrows
DELETE FROM activity_entry;

-- name: ImportMediaObject :execrows
INSERT INTO media_object (
  id,
  tenant_id,
  storage_key,
  mime_type,
  byte_size,
  checksum,
  usage,
  ref_count,
  created_by,
  created_at,
  deleted_at,
  status,
  file_name
)
SELECT
  r.id,
  r.tenant_id,
  r.storage_key,
  r.mime_type,
  r.byte_size,
  r.checksum,
  r.usage,
  r.ref_count,
  r.created_by,
  r.created_at,
  r.deleted_at,
  r.status,
  r.file_name
FROM jsonb_populate_record(
  NULL::media_object,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  storage_key = EXCLUDED.storage_key,
  mime_type = EXCLUDED.mime_type,
  byte_size = EXCLUDED.byte_size,
  checksum = EXCLUDED.checksum,
  usage = EXCLUDED.usage,
  ref_count = EXCLUDED.ref_count,
  created_by = EXCLUDED.created_by,
  created_at = EXCLUDED.created_at,
  deleted_at = EXCLUDED.deleted_at,
  status = EXCLUDED.status,
  file_name = EXCLUDED.file_name
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsMediaObject :one
SELECT EXISTS (SELECT 1 FROM media_object WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearMediaObject :execrows
DELETE FROM media_object;

-- name: ImportItemAttachment :execrows
INSERT INTO item_attachment (
  tenant_id,
  item_id,
  media_id
)
SELECT
  r.tenant_id,
  r.item_id,
  r.media_id
FROM jsonb_populate_record(
  NULL::item_attachment,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (item_id, media_id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsItemAttachment :one
SELECT EXISTS (SELECT 1 FROM item_attachment WHERE item_id = (sqlc.arg('payload')::jsonb->>'item_id')::uuid AND media_id = (sqlc.arg('payload')::jsonb->>'media_id')::uuid) AS held;

-- name: ClearItemAttachment :execrows
DELETE FROM item_attachment;

-- name: ImportRecurrenceRule :execrows
INSERT INTO recurrence_rule (
  id,
  tenant_id,
  source_item_id,
  rrule,
  time_zone,
  mode,
  horizon_days,
  ends_at,
  max_count,
  last_materialized_at,
  created_at,
  version,
  updated_at
)
SELECT
  r.id,
  r.tenant_id,
  r.source_item_id,
  r.rrule,
  r.time_zone,
  r.mode,
  r.horizon_days,
  r.ends_at,
  r.max_count,
  r.last_materialized_at,
  r.created_at,
  r.version,
  r.updated_at
FROM jsonb_populate_record(
  NULL::recurrence_rule,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  source_item_id = EXCLUDED.source_item_id,
  rrule = EXCLUDED.rrule,
  time_zone = EXCLUDED.time_zone,
  mode = EXCLUDED.mode,
  horizon_days = EXCLUDED.horizon_days,
  ends_at = EXCLUDED.ends_at,
  max_count = EXCLUDED.max_count,
  last_materialized_at = EXCLUDED.last_materialized_at,
  created_at = EXCLUDED.created_at,
  version = EXCLUDED.version,
  updated_at = EXCLUDED.updated_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsRecurrenceRule :one
SELECT EXISTS (SELECT 1 FROM recurrence_rule WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearRecurrenceRule :execrows
DELETE FROM recurrence_rule;

-- name: ImportReminder :execrows
INSERT INTO reminder (
  id,
  tenant_id,
  item_id,
  offset_spec,
  channels,
  recipients,
  state,
  fire_at,
  created_at,
  version,
  updated_at
)
SELECT
  r.id,
  r.tenant_id,
  r.item_id,
  r.offset_spec,
  r.channels,
  r.recipients,
  r.state,
  r.fire_at,
  r.created_at,
  r.version,
  r.updated_at
FROM jsonb_populate_record(
  NULL::reminder,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  item_id = EXCLUDED.item_id,
  offset_spec = EXCLUDED.offset_spec,
  channels = EXCLUDED.channels,
  recipients = EXCLUDED.recipients,
  state = EXCLUDED.state,
  fire_at = EXCLUDED.fire_at,
  created_at = EXCLUDED.created_at,
  version = EXCLUDED.version,
  updated_at = EXCLUDED.updated_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsReminder :one
SELECT EXISTS (SELECT 1 FROM reminder WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearReminder :execrows
DELETE FROM reminder;

-- name: ImportSavedView :execrows
INSERT INTO saved_view (
  id,
  tenant_id,
  scope_type,
  scope_id,
  owner_id,
  name,
  layout,
  query,
  grouping,
  visible_fields,
  sharing,
  created_at,
  version
)
SELECT
  r.id,
  r.tenant_id,
  r.scope_type,
  r.scope_id,
  r.owner_id,
  r.name,
  r.layout,
  r.query,
  r.grouping,
  r.visible_fields,
  r.sharing,
  r.created_at,
  r.version
FROM jsonb_populate_record(
  NULL::saved_view,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  scope_type = EXCLUDED.scope_type,
  scope_id = EXCLUDED.scope_id,
  owner_id = EXCLUDED.owner_id,
  name = EXCLUDED.name,
  layout = EXCLUDED.layout,
  query = EXCLUDED.query,
  grouping = EXCLUDED.grouping,
  visible_fields = EXCLUDED.visible_fields,
  sharing = EXCLUDED.sharing,
  created_at = EXCLUDED.created_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsSavedView :one
SELECT EXISTS (SELECT 1 FROM saved_view WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearSavedView :execrows
DELETE FROM saved_view;

-- name: ImportTemplate :execrows
INSERT INTO template (
  id,
  tenant_id,
  scope_type,
  scope_id,
  name,
  description,
  root_type,
  nodes,
  created_at,
  deleted_at,
  version,
  updated_at
)
SELECT
  r.id,
  r.tenant_id,
  r.scope_type,
  r.scope_id,
  r.name,
  r.description,
  r.root_type,
  r.nodes,
  r.created_at,
  r.deleted_at,
  r.version,
  r.updated_at
FROM jsonb_populate_record(
  NULL::template,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  scope_type = EXCLUDED.scope_type,
  scope_id = EXCLUDED.scope_id,
  name = EXCLUDED.name,
  description = EXCLUDED.description,
  root_type = EXCLUDED.root_type,
  nodes = EXCLUDED.nodes,
  created_at = EXCLUDED.created_at,
  deleted_at = EXCLUDED.deleted_at,
  version = EXCLUDED.version,
  updated_at = EXCLUDED.updated_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsTemplate :one
SELECT EXISTS (SELECT 1 FROM template WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearTemplate :execrows
DELETE FROM template;

-- name: ImportJumbleEntry :execrows
INSERT INTO jumble_entry (
  id,
  tenant_id,
  channel,
  sender,
  raw_subject,
  raw_body,
  attachments,
  suggestion,
  status,
  target_item_id,
  received_at,
  processed_at
)
SELECT
  r.id,
  r.tenant_id,
  r.channel,
  r.sender,
  r.raw_subject,
  r.raw_body,
  r.attachments,
  r.suggestion,
  r.status,
  r.target_item_id,
  r.received_at,
  r.processed_at
FROM jsonb_populate_record(
  NULL::jumble_entry,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  channel = EXCLUDED.channel,
  sender = EXCLUDED.sender,
  raw_subject = EXCLUDED.raw_subject,
  raw_body = EXCLUDED.raw_body,
  attachments = EXCLUDED.attachments,
  suggestion = EXCLUDED.suggestion,
  status = EXCLUDED.status,
  target_item_id = EXCLUDED.target_item_id,
  received_at = EXCLUDED.received_at,
  processed_at = EXCLUDED.processed_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsJumbleEntry :one
SELECT EXISTS (SELECT 1 FROM jumble_entry WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearJumbleEntry :execrows
DELETE FROM jumble_entry;

-- name: ImportAutoAssignPolicy :execrows
INSERT INTO auto_assign_policy (
  id,
  tenant_id,
  scope_type,
  scope_id,
  strategy,
  candidates,
  state,
  enabled,
  version
)
SELECT
  r.id,
  r.tenant_id,
  r.scope_type,
  r.scope_id,
  r.strategy,
  r.candidates,
  r.state,
  r.enabled,
  r.version
FROM jsonb_populate_record(
  NULL::auto_assign_policy,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  scope_type = EXCLUDED.scope_type,
  scope_id = EXCLUDED.scope_id,
  strategy = EXCLUDED.strategy,
  candidates = EXCLUDED.candidates,
  state = EXCLUDED.state,
  enabled = EXCLUDED.enabled,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsAutoAssignPolicy :one
SELECT EXISTS (SELECT 1 FROM auto_assign_policy WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearAutoAssignPolicy :execrows
DELETE FROM auto_assign_policy;

-- name: ImportAutomationRule :execrows
INSERT INTO automation_rule (
  id,
  tenant_id,
  scope_type,
  scope_id,
  name,
  enabled,
  run_as,
  trigger,
  conditions,
  actions,
  throttle,
  on_error,
  failure_count,
  created_by,
  created_at,
  updated_at,
  deleted_at,
  version,
  -- Restored with the rule, so that a rule that comes back is the rule that went in. It fires
  -- nothing by itself: a restore seeds no poller, because nothing enumerates tenants, and
  -- backup-restore.md §8.4 is unambiguous that no rule acts because of a restore.
  next_run_at
)
SELECT
  r.id,
  r.tenant_id,
  r.scope_type,
  r.scope_id,
  r.name,
  r.enabled,
  r.run_as,
  r.trigger,
  r.conditions,
  r.actions,
  r.throttle,
  r.on_error,
  r.failure_count,
  r.created_by,
  r.created_at,
  r.updated_at,
  r.deleted_at,
  r.version,
  r.next_run_at
FROM jsonb_populate_record(
  NULL::automation_rule,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  scope_type = EXCLUDED.scope_type,
  scope_id = EXCLUDED.scope_id,
  name = EXCLUDED.name,
  enabled = EXCLUDED.enabled,
  run_as = EXCLUDED.run_as,
  trigger = EXCLUDED.trigger,
  conditions = EXCLUDED.conditions,
  actions = EXCLUDED.actions,
  throttle = EXCLUDED.throttle,
  on_error = EXCLUDED.on_error,
  failure_count = EXCLUDED.failure_count,
  created_by = EXCLUDED.created_by,
  created_at = EXCLUDED.created_at,
  updated_at = EXCLUDED.updated_at,
  deleted_at = EXCLUDED.deleted_at,
  version = EXCLUDED.version,
  next_run_at = EXCLUDED.next_run_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsAutomationRule :one
SELECT EXISTS (SELECT 1 FROM automation_rule WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearAutomationRule :execrows
DELETE FROM automation_rule;

-- name: ImportWebhookSubscription :execrows
INSERT INTO webhook_subscription (
  id,
  tenant_id,
  target_url,
  event_types,
  filter_expr,
  secret_enc,
  state,
  failure_count,
  created_by,
  created_at,
  version
)
SELECT
  r.id,
  r.tenant_id,
  r.target_url,
  r.event_types,
  r.filter_expr,
  r.secret_enc,
  r.state,
  r.failure_count,
  r.created_by,
  r.created_at,
  r.version
FROM jsonb_populate_record(
  NULL::webhook_subscription,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  target_url = EXCLUDED.target_url,
  event_types = EXCLUDED.event_types,
  filter_expr = EXCLUDED.filter_expr,
  secret_enc = EXCLUDED.secret_enc,
  state = EXCLUDED.state,
  failure_count = EXCLUDED.failure_count,
  created_by = EXCLUDED.created_by,
  created_at = EXCLUDED.created_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsWebhookSubscription :one
SELECT EXISTS (SELECT 1 FROM webhook_subscription WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearWebhookSubscription :execrows
DELETE FROM webhook_subscription;

-- name: ImportCalendarFeed :execrows
INSERT INTO calendar_feed (
  id,
  tenant_id,
  account_id,
  view_id,
  token_hash,
  created_at,
  revoked_at
)
SELECT
  r.id,
  r.tenant_id,
  r.account_id,
  r.view_id,
  r.token_hash,
  r.created_at,
  r.revoked_at
FROM jsonb_populate_record(
  NULL::calendar_feed,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  account_id = EXCLUDED.account_id,
  view_id = EXCLUDED.view_id,
  token_hash = EXCLUDED.token_hash,
  created_at = EXCLUDED.created_at,
  revoked_at = EXCLUDED.revoked_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCalendarFeed :one
SELECT EXISTS (SELECT 1 FROM calendar_feed WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCalendarFeed :execrows
DELETE FROM calendar_feed;

-- name: ImportNotificationPreference :execrows
INSERT INTO notification_preference (
  tenant_id,
  account_id,
  category,
  channel,
  enabled,
  include_title,
  updated_at
)
SELECT
  r.tenant_id,
  r.account_id,
  r.category,
  r.channel,
  r.enabled,
  r.include_title,
  r.updated_at
FROM jsonb_populate_record(
  NULL::notification_preference,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, account_id, category, channel) DO UPDATE SET
  enabled = EXCLUDED.enabled,
  include_title = EXCLUDED.include_title,
  updated_at = EXCLUDED.updated_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsNotificationPreference :one
SELECT EXISTS (SELECT 1 FROM notification_preference WHERE account_id = (sqlc.arg('payload')::jsonb->>'account_id')::uuid AND category = (sqlc.arg('payload')::jsonb->>'category')::text AND channel = (sqlc.arg('payload')::jsonb->>'channel')::text) AS held;

-- name: ClearNotificationPreference :execrows
DELETE FROM notification_preference;

-- name: ImportRetentionPolicy :execrows
INSERT INTO retention_policy (
  tenant_id,
  data_kind,
  retain_days,
  min_days,
  max_days,
  justification,
  updated_at,
  updated_by
)
SELECT
  r.tenant_id,
  r.data_kind,
  r.retain_days,
  r.min_days,
  r.max_days,
  r.justification,
  r.updated_at,
  r.updated_by
FROM jsonb_populate_record(
  NULL::retention_policy,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, data_kind) DO UPDATE SET
  retain_days = EXCLUDED.retain_days,
  min_days = EXCLUDED.min_days,
  max_days = EXCLUDED.max_days,
  justification = EXCLUDED.justification,
  updated_at = EXCLUDED.updated_at,
  updated_by = EXCLUDED.updated_by
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsRetentionPolicy :one
SELECT EXISTS (SELECT 1 FROM retention_policy WHERE data_kind = (sqlc.arg('payload')::jsonb->>'data_kind')::text) AS held;

-- name: ClearRetentionPolicy :execrows
DELETE FROM retention_policy;

-- name: ImportConsentRecord :execrows
INSERT INTO consent_record (
  id,
  tenant_id,
  account_id,
  purpose,
  granted,
  granted_at,
  revoked_at,
  source
)
SELECT
  r.id,
  r.tenant_id,
  r.account_id,
  r.purpose,
  r.granted,
  r.granted_at,
  r.revoked_at,
  r.source
FROM jsonb_populate_record(
  NULL::consent_record,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  account_id = EXCLUDED.account_id,
  purpose = EXCLUDED.purpose,
  granted = EXCLUDED.granted,
  granted_at = EXCLUDED.granted_at,
  revoked_at = EXCLUDED.revoked_at,
  source = EXCLUDED.source
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsConsentRecord :one
SELECT EXISTS (SELECT 1 FROM consent_record WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearConsentRecord :execrows
DELETE FROM consent_record;

-- name: ImportLegalHold :execrows
INSERT INTO legal_hold (
  id,
  tenant_id,
  scope_kind,
  scope_id,
  reason,
  placed_by,
  placed_at,
  released_by,
  released_at
)
SELECT
  r.id,
  r.tenant_id,
  r.scope_kind,
  r.scope_id,
  r.reason,
  r.placed_by,
  r.placed_at,
  r.released_by,
  r.released_at
FROM jsonb_populate_record(
  NULL::legal_hold,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  scope_kind = EXCLUDED.scope_kind,
  scope_id = EXCLUDED.scope_id,
  reason = EXCLUDED.reason,
  placed_by = EXCLUDED.placed_by,
  placed_at = EXCLUDED.placed_at,
  released_by = EXCLUDED.released_by,
  released_at = EXCLUDED.released_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsLegalHold :one
SELECT EXISTS (SELECT 1 FROM legal_hold WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearLegalHold :execrows
DELETE FROM legal_hold;

-- name: ImportSetElement :execrows
INSERT INTO set_element (
  tenant_id,
  item_id,
  set_name,
  element_id,
  add_tag,
  remove_tag
)
SELECT
  r.tenant_id,
  r.item_id,
  r.set_name,
  r.element_id,
  r.add_tag,
  r.remove_tag
FROM jsonb_populate_record(
  NULL::set_element,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, item_id, set_name, element_id) DO UPDATE SET
  add_tag = EXCLUDED.add_tag,
  remove_tag = EXCLUDED.remove_tag
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsSetElement :one
SELECT EXISTS (SELECT 1 FROM set_element WHERE item_id = (sqlc.arg('payload')::jsonb->>'item_id')::uuid AND set_name = (sqlc.arg('payload')::jsonb->>'set_name')::text AND element_id = (sqlc.arg('payload')::jsonb->>'element_id')::uuid) AS held;

-- name: ClearSetElement :execrows
DELETE FROM set_element;
