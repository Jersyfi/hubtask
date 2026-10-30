-- The statements a restore writes a tenant through (backup-restore.md §8).
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
-- ai_consent is neither written nor updated: an archive from before tenant-export.md §11 still
-- carries it, jsonb_populate_record accepts the key, and the column keeps what it holds.
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
  sign_in_name,
  start_password_at,
  terms_url,
  terms_version,
  terms_agreed_at,
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
  r.sign_in_name,
  r.start_password_at,
  r.terms_url,
  r.terms_version,
  r.terms_agreed_at,
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
  sign_in_name = EXCLUDED.sign_in_name,
  start_password_at = EXCLUDED.start_password_at,
  terms_url = EXCLUDED.terms_url,
  terms_version = EXCLUDED.terms_version,
  terms_agreed_at = EXCLUDED.terms_agreed_at,
  created_at = EXCLUDED.created_at,
  updated_at = EXCLUDED.updated_at,
  deleted_at = EXCLUDED.deleted_at,
  version = EXCLUDED.version
-- A restricted account is neither deleted nor changed (data-retention.md §4 item 2): the archive's
-- version of it would lift a restriction of processing - one a legal hold may be keeping - by going
-- back in time, which only a deliberate act may do (Art. 18). The same for an account an erasure
-- keeps a part of under a hold, which carries its restriction in the kept record, not its status.
WHERE sqlc.arg('overwrite')::boolean AND account.status <> 'RESTRICTED'
  AND NOT EXISTS (
    SELECT 1 FROM erasure_kept k
    JOIN data_subject_request r ON r.tenant_id = k.tenant_id AND r.id = k.request_id
    WHERE k.tenant_id = account.tenant_id AND k.erased_at IS NULL
      AND r.subject_account_id = account.id);

-- name: HoldsAccount :one
SELECT EXISTS (SELECT 1 FROM account WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearAccount :execrows
-- A replace keeps a restricted account, and one an erasure keeps a part of, for ImportAccount's
-- reason: the archive's row then meets it and leaves it as it is.
DELETE FROM account a WHERE a.status <> 'RESTRICTED'
  AND NOT EXISTS (
    SELECT 1 FROM erasure_kept k
    JOIN data_subject_request r ON r.tenant_id = k.tenant_id AND r.id = k.request_id
    WHERE k.tenant_id = a.tenant_id AND k.erased_at IS NULL AND r.subject_account_id = a.id);

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
-- A row equal to a membership the tenant holds under another identifier is neither inserted nor,
-- under OVERWRITE, written over the row of its own identifier: a subject holds a role at a scope
-- once (0149), and the index is no arbiter of the upsert. An archive from before 0149 can carry
-- equal rows, and MERGE can meet a role granted again since; the row is reported as skipped rather
-- than failing the restore. The export orders memberships by identifier, so the oldest arrives
-- first and is the one kept, as 0149 keeps it.
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
WHERE NOT EXISTS (
  SELECT 1 FROM membership m
  WHERE m.id <> r.id
    AND m.account_id IS NOT DISTINCT FROM r.account_id
    AND m.group_id IS NOT DISTINCT FROM r.group_id
    AND m.scope_type = r.scope_type
    AND m.scope_id IS NOT DISTINCT FROM r.scope_id
    AND m.role = r.role
)
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
  deleted_by_type,
  deleted_by_id,
  created_by,
  created_at,
  updated_at,
  version,
  private,
  handle,
  item_counter
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
  r.deleted_by_type,
  r.deleted_by_id,
  r.created_by,
  r.created_at,
  r.updated_at,
  r.version,
  -- An archive written before 0124 carries no flag, and its hubs were shared when it was taken
  -- (tenant-export.md §11: an additive field keeps the format).
  coalesce(r.private, false),
  -- A collection's handle and count travel with it (migration 0204); an archive written before
  -- carries neither, and the store then gives the handle and the count starts at none.
  r.handle,
  coalesce(r.item_counter, 0)
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
  deleted_by_type = EXCLUDED.deleted_by_type,
  deleted_by_id = EXCLUDED.deleted_by_id,
  created_by = EXCLUDED.created_by,
  created_at = EXCLUDED.created_at,
  updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version,
  private = EXCLUDED.private,
  handle = EXCLUDED.handle,
  item_counter = greatest(container.item_counter, EXCLUDED.item_counter)
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
  currency,
  formula,
  is_required,
  restricted,
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
  r.currency,
  r.formula,
  r.is_required,
  coalesce(r.restricted, false),
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
  currency = EXCLUDED.currency,
  formula = EXCLUDED.formula,
  restricted = EXCLUDED.restricted,
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
  recurrence_source_id,
  origin_jumble_id,
  content_language,
  calendar_uid,
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
  retention_blocked_by,
  amount,
  completion_count,
  flow_started_at,
  flow_finished_at,
  number,
  entry_type,
  status
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
  r.recurrence_source_id,
  r.origin_jumble_id,
  r.content_language,
  r.calendar_uid,
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
  r.retention_blocked_by,
  r.amount,
  -- An archive written before migration 0189 carries no count, and the column takes no NULL.
  coalesce(r.completion_count, 0),
  r.flow_started_at,
  r.flow_finished_at,
  -- An entry's number travels with it (migration 0204); an archive written before carries none,
  -- and the store then draws the next of its collection.
  r.number,
  -- An archive written before migration 0209 carries none: the level's system type.
  r.entry_type,
  -- An archive written before migration 0216 carries none: no workflow ran.
  r.status
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
  recurrence_source_id = EXCLUDED.recurrence_source_id,
  origin_jumble_id = EXCLUDED.origin_jumble_id,
  content_language = EXCLUDED.content_language,
  calendar_uid = EXCLUDED.calendar_uid,
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
  retention_blocked_by = EXCLUDED.retention_blocked_by,
  amount = EXCLUDED.amount,
  completion_count = EXCLUDED.completion_count,
  flow_started_at = EXCLUDED.flow_started_at,
  flow_finished_at = EXCLUDED.flow_finished_at,
  number = EXCLUDED.number,
  entry_type = EXCLUDED.entry_type,
  status = EXCLUDED.status
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
  version,
  kind,
  system_code,
  system_params,
  internal
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
  r.version,
  -- An archive written before migration 0084 carries no kind: what somebody wrote.
  coalesce(r.kind, 'USER'),
  r.system_code,
  r.system_params,
  -- An archive written before migration 0241 carries no flag: a public comment.
  coalesce(r.internal, false)
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
  version = EXCLUDED.version,
  kind = EXCLUDED.kind,
  system_code = EXCLUDED.system_code,
  system_params = EXCLUDED.system_params,
  internal = EXCLUDED.internal
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
-- The conflict target is the partitioned key: a partitioned table cannot hold a
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
  processed_at,
  created_by
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
  r.processed_at,
  r.created_by
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
  processed_at = EXCLUDED.processed_at,
  created_by = EXCLUDED.created_by
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
  scope_type,
  scope_id,
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
  r.scope_type,
  r.scope_id,
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
  scope_type = EXCLUDED.scope_type,
  scope_id = EXCLUDED.scope_id,
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
  released_at,
  released_reason
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
  r.released_at,
  r.released_reason
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
  released_at = EXCLUDED.released_at,
  released_reason = EXCLUDED.released_reason
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsLegalHold :one
SELECT EXISTS (SELECT 1 FROM legal_hold WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearLegalHold :execrows
DELETE FROM legal_hold;

-- name: ImportPasskey :execrows
-- Skipped rather than refused: a passkey whose account the restore did not bring back, one whose
-- identifier another row of the workspace holds, one past the account's ten, and one from an
-- export, which carries no identifier or key (tenant-export.md §9) and so is no credential.
INSERT INTO webauthn_credential (
  id,
  tenant_id,
  account_id,
  credential_id,
  public_key,
  algorithm,
  sign_count,
  user_handle,
  backup_eligible,
  backed_up,
  transports,
  name,
  created_at,
  last_used_at,
  rp_id
)
SELECT
  r.id,
  r.tenant_id,
  r.account_id,
  r.credential_id,
  r.public_key,
  r.algorithm,
  r.sign_count,
  r.user_handle,
  r.backup_eligible,
  r.backed_up,
  r.transports,
  r.name,
  r.created_at,
  r.last_used_at,
  r.rp_id
FROM jsonb_populate_record(
  NULL::webauthn_credential,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
WHERE r.credential_id IS NOT NULL AND r.public_key IS NOT NULL AND r.user_handle IS NOT NULL
  AND EXISTS (SELECT 1 FROM account a WHERE a.id = r.account_id)
  AND NOT EXISTS (
    SELECT 1 FROM webauthn_credential w WHERE w.credential_id = r.credential_id AND w.id <> r.id
  )
  AND (SELECT count(*) FROM webauthn_credential w WHERE w.account_id = r.account_id AND w.id <> r.id) < 10
ON CONFLICT (id) DO UPDATE SET
  account_id = EXCLUDED.account_id,
  credential_id = EXCLUDED.credential_id,
  public_key = EXCLUDED.public_key,
  algorithm = EXCLUDED.algorithm,
  sign_count = EXCLUDED.sign_count,
  user_handle = EXCLUDED.user_handle,
  backup_eligible = EXCLUDED.backup_eligible,
  backed_up = EXCLUDED.backed_up,
  transports = EXCLUDED.transports,
  name = EXCLUDED.name,
  created_at = EXCLUDED.created_at,
  last_used_at = EXCLUDED.last_used_at,
  rp_id = EXCLUDED.rp_id
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsPasskey :one
SELECT EXISTS (SELECT 1 FROM webauthn_credential WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearPasskeys :execrows
DELETE FROM webauthn_credential;

-- name: ImportPrivateHubAccess :execrows
-- An access whose hub the restore did not bring back has nothing to describe, and is skipped
-- rather than refused by the foreign key.
INSERT INTO private_hub_access (
  id,
  tenant_id,
  hub_id,
  opened_by,
  reason,
  opened_at,
  ends_at
)
SELECT
  r.id,
  r.tenant_id,
  r.hub_id,
  r.opened_by,
  r.reason,
  r.opened_at,
  r.ends_at
FROM jsonb_populate_record(
  NULL::private_hub_access,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
WHERE EXISTS (SELECT 1 FROM container c WHERE c.id = r.hub_id)
ON CONFLICT (id) DO UPDATE SET
  hub_id = EXCLUDED.hub_id,
  opened_by = EXCLUDED.opened_by,
  reason = EXCLUDED.reason,
  opened_at = EXCLUDED.opened_at,
  ends_at = EXCLUDED.ends_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsPrivateHubAccess :one
SELECT EXISTS (SELECT 1 FROM private_hub_access WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearPrivateHubAccess :execrows
DELETE FROM private_hub_access;

-- name: ImportContainerMilestone :execrows
-- With its moment and its container's identity, as a person saw it. A milestone whose container the
-- restore did not bring back marks nothing, and is skipped rather than refused by the foreign key.
INSERT INTO container_milestone (
  tenant_id,
  container_id,
  reached_at
)
SELECT
  r.tenant_id,
  r.container_id,
  r.reached_at
FROM jsonb_populate_record(
  NULL::container_milestone,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
WHERE EXISTS (SELECT 1 FROM container c WHERE c.id = r.container_id)
ON CONFLICT (tenant_id, container_id) DO UPDATE SET
  reached_at = EXCLUDED.reached_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsContainerMilestone :one
SELECT EXISTS (SELECT 1 FROM container_milestone WHERE container_id = (sqlc.arg('payload')::jsonb->>'container_id')::uuid) AS held;

-- name: ClearContainerMilestone :execrows
DELETE FROM container_milestone;

-- name: ImportStructurePreset :execrows
-- A person's own preset whose account the restore did not bring back belongs to nobody, and is
-- skipped rather than refused by the foreign key.
INSERT INTO structure_preset (
  tenant_id,
  id,
  origin,
  owner_id,
  name,
  settings,
  created_at,
  version
)
SELECT
  r.tenant_id,
  r.id,
  r.origin,
  r.owner_id,
  r.name,
  r.settings,
  r.created_at,
  r.version
FROM jsonb_populate_record(
  NULL::structure_preset,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
WHERE r.owner_id IS NULL OR EXISTS (SELECT 1 FROM account a WHERE a.id = r.owner_id)
ON CONFLICT (tenant_id, id) DO UPDATE SET
  origin = EXCLUDED.origin,
  owner_id = EXCLUDED.owner_id,
  name = EXCLUDED.name,
  settings = EXCLUDED.settings,
  created_at = EXCLUDED.created_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsStructurePreset :one
SELECT EXISTS (SELECT 1 FROM structure_preset WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearStructurePreset :execrows
DELETE FROM structure_preset;

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

-- name: ImportSettingValue :execrows
-- A workspace's own entries are keyed by the workspace itself, which a restore into another
-- workspace is: the scope follows the tenant written into.
INSERT INTO setting_value (id, tenant_id, level, scope_id, key, value, hold, allowed, set_by, updated_at)
SELECT
  r.id,
  r.tenant_id,
  r.level,
  CASE WHEN r.level = 'WORKSPACE' THEN current_tenant_id() ELSE r.scope_id END,
  r.key,
  r.value,
  r.hold,
  r.allowed,
  r.set_by,
  r.updated_at
FROM jsonb_populate_record(
  NULL::setting_value,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  value = EXCLUDED.value,
  hold = EXCLUDED.hold,
  allowed = EXCLUDED.allowed,
  set_by = EXCLUDED.set_by,
  updated_at = EXCLUDED.updated_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsSettingValue :one
SELECT EXISTS (SELECT 1 FROM setting_value WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearSettingValue :execrows
DELETE FROM setting_value;

-- name: ImportEntryType :execrows
INSERT INTO entry_type (
  id, tenant_id, key, level, system, name, icon, color_token, completable, workflow, record,
  schedulable, estimable, allowed_children, form, position, archived_at, created_at, updated_at, version
)
SELECT
  -- An archive from before 0214 carries no form: the type then has the empty one.
  r.id, r.tenant_id, r.key, r.level, r.system, r.name, r.icon, r.color_token, r.completable,
  r.workflow, r.record, r.schedulable, r.estimable, r.allowed_children, COALESCE(r.form, '{}'::jsonb),
  r.position, r.archived_at,
  r.created_at, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::entry_type,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  icon = EXCLUDED.icon,
  color_token = EXCLUDED.color_token,
  completable = EXCLUDED.completable,
  workflow = EXCLUDED.workflow,
  record = EXCLUDED.record,
  schedulable = EXCLUDED.schedulable,
  estimable = EXCLUDED.estimable,
  allowed_children = EXCLUDED.allowed_children,
  form = EXCLUDED.form,
  position = EXCLUDED.position,
  archived_at = EXCLUDED.archived_at,
  updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsEntryType :one
SELECT EXISTS (SELECT 1 FROM entry_type WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearEntryType :execrows
DELETE FROM entry_type;

-- name: ImportCustomRole :execrows
INSERT INTO custom_role (id, tenant_id, key, name, permissions, created_at, updated_at, version)
SELECT r.id, r.tenant_id, r.key, r.name, r.permissions, r.created_at, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::custom_role,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  permissions = EXCLUDED.permissions,
  updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCustomRole :one
SELECT EXISTS (SELECT 1 FROM custom_role WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCustomRole :execrows
DELETE FROM custom_role;

-- name: ImportCustomRoleGrant :execrows
-- A grant is given and taken back, never changed.
INSERT INTO custom_role_grant (id, tenant_id, account_id, group_id, scope_type, scope_id, role_id, created_at)
SELECT r.id, r.tenant_id, r.account_id, r.group_id, r.scope_type, r.scope_id, r.role_id, r.created_at
FROM jsonb_populate_record(
  NULL::custom_role_grant,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT DO NOTHING;

-- name: HoldsCustomRoleGrant :one
SELECT EXISTS (SELECT 1 FROM custom_role_grant WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCustomRoleGrant :execrows
DELETE FROM custom_role_grant;

-- name: ImportConfigRevision :execrows
-- A version is evidence, written once: one the workspace holds is kept.
INSERT INTO config_revision (
  tenant_id, version, document, hash, plan_hash, actor_kind, actor_id, proposed_by, rolled_back_to,
  template, added, changed, removed, created_at, hub_id
)
SELECT r.tenant_id, r.version, r.document, r.hash, r.plan_hash, r.actor_kind, r.actor_id, r.proposed_by,
       r.rolled_back_to, r.template, r.added, r.changed, r.removed, r.created_at, r.hub_id
FROM jsonb_populate_record(
  NULL::config_revision,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT DO NOTHING;

-- name: HoldsConfigRevision :one
SELECT EXISTS (
  SELECT 1 FROM config_revision WHERE version = (sqlc.arg('payload')::jsonb->>'version')::integer
) AS held;

-- name: ClearConfigRevision :execrows
DELETE FROM config_revision;

-- name: ImportConfigProposal :execrows
INSERT INTO config_proposal (
  id, tenant_id, state, document, mode, base_hash, plan_hash, template, added, changed, removed,
  hides_data, changes_permissions, proposed_by, decided_by, created_at, decided_at, hub_id
)
SELECT r.id, r.tenant_id, r.state, r.document, r.mode, r.base_hash, r.plan_hash, r.template, r.added,
       r.changed, r.removed, r.hides_data, r.changes_permissions, r.proposed_by, r.decided_by,
       r.created_at, r.decided_at, r.hub_id
FROM jsonb_populate_record(
  NULL::config_proposal,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  state = EXCLUDED.state,
  decided_by = EXCLUDED.decided_by,
  decided_at = EXCLUDED.decided_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsConfigProposal :one
SELECT EXISTS (SELECT 1 FROM config_proposal WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearConfigProposal :execrows
DELETE FROM config_proposal;

-- name: ImportConfigTemplate :execrows
-- A template's version is never rewritten: one the workspace holds, by identity or by key and
-- version, is kept.
INSERT INTO config_template (id, tenant_id, key, version, name, origin, document, created_by, created_at, hub_document)
SELECT r.id, r.tenant_id, r.key, r.version, r.name, r.origin, r.document, r.created_by, r.created_at, r.hub_document
FROM jsonb_populate_record(
  NULL::config_template,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT DO NOTHING;

-- name: HoldsConfigTemplate :one
SELECT EXISTS (SELECT 1 FROM config_template WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearConfigTemplate :execrows
DELETE FROM config_template;

-- name: ImportConfigTemplateUse :execrows
INSERT INTO config_template_use (tenant_id, template_key, version, document, applied_at)
SELECT r.tenant_id, r.template_key, r.version, r.document, r.applied_at
FROM jsonb_populate_record(
  NULL::config_template_use,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, template_key) DO UPDATE SET
  version = EXCLUDED.version,
  document = EXCLUDED.document,
  applied_at = EXCLUDED.applied_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsConfigTemplateUse :one
SELECT EXISTS (
  SELECT 1 FROM config_template_use WHERE template_key = sqlc.arg('payload')::jsonb->>'template_key'
) AS held;

-- name: ClearConfigTemplateUse :execrows
DELETE FROM config_template_use;

-- name: ImportConfigHubTemplateUse :execrows
INSERT INTO config_hub_template_use (tenant_id, hub_id, template_key, version, document, applied_at)
SELECT r.tenant_id, r.hub_id, r.template_key, r.version, r.document, r.applied_at
FROM jsonb_populate_record(
  NULL::config_hub_template_use,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, hub_id, template_key) DO UPDATE SET
  version = EXCLUDED.version,
  document = EXCLUDED.document,
  applied_at = EXCLUDED.applied_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsConfigHubTemplateUse :one
SELECT EXISTS (
  SELECT 1 FROM config_hub_template_use
  WHERE hub_id = (sqlc.arg('payload')::jsonb->>'hub_id')::uuid
    AND template_key = sqlc.arg('payload')::jsonb->>'template_key'
) AS held;

-- name: ClearConfigHubTemplateUse :execrows
DELETE FROM config_hub_template_use;

-- name: ImportLinkType :execrows
INSERT INTO link_type (
  id, tenant_id, key, name, inverse_name, direction, cardinality, cycles, waits, position,
  archived_at, created_at, updated_at, version
)
SELECT
  r.id, r.tenant_id, r.key, r.name, r.inverse_name, r.direction, r.cardinality, r.cycles, r.waits,
  r.position, r.archived_at, r.created_at, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::link_type,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  inverse_name = EXCLUDED.inverse_name,
  position = EXCLUDED.position,
  archived_at = EXCLUDED.archived_at,
  updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsLinkType :one
SELECT EXISTS (SELECT 1 FROM link_type WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearLinkType :execrows
DELETE FROM link_type;

-- name: ImportEntryLink :execrows
-- A link is drawn and removed, never changed: an overwrite replaces only its timing - the one
-- part the application may update (migration 0244's grant); naming any other column in the SET
-- makes every write of a link a permission refusal. An archive written before migration 0244
-- holds no timing: the columns' defaults stand.
INSERT INTO entry_link (id, tenant_id, link_type, source_id, target_id, created_by, created_at,
  dependency_kind, lag_days)
SELECT r.id, r.tenant_id, r.link_type, r.source_id, r.target_id, r.created_by, r.created_at,
  coalesce(r.dependency_kind, 'FS'), coalesce(r.lag_days, 0)
FROM jsonb_populate_record(
  NULL::entry_link,
  (sqlc.arg('payload')::jsonb - 'own_type') || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  dependency_kind = EXCLUDED.dependency_kind, lag_days = EXCLUDED.lag_days
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsEntryLink :one
SELECT EXISTS (SELECT 1 FROM entry_link WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearEntryLink :execrows
DELETE FROM entry_link;

-- name: ImportWorkflow :execrows
INSERT INTO workflow (
  id, tenant_id, key, system, name, published, draft, published_at, published_by,
  created_at, updated_at, version
)
SELECT
  r.id, r.tenant_id, r.key, r.system, r.name, r.published, r.draft, r.published_at, r.published_by,
  r.created_at, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::workflow,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  published = EXCLUDED.published,
  draft = EXCLUDED.draft,
  published_at = EXCLUDED.published_at,
  published_by = EXCLUDED.published_by,
  updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsWorkflow :one
SELECT EXISTS (SELECT 1 FROM workflow WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearWorkflow :execrows
DELETE FROM workflow;

-- name: ImportWorkflowScheme :execrows
INSERT INTO workflow_scheme (tenant_id, container_id, entry_type, workflow_key, created_at)
SELECT r.tenant_id, r.container_id, r.entry_type, r.workflow_key, r.created_at
FROM jsonb_populate_record(
  NULL::workflow_scheme,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, container_id, entry_type) DO UPDATE SET
  workflow_key = EXCLUDED.workflow_key
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsWorkflowScheme :one
SELECT EXISTS (
  SELECT 1 FROM workflow_scheme
  WHERE container_id = (sqlc.arg('payload')::jsonb->>'container_id')::uuid
    AND entry_type = sqlc.arg('payload')::jsonb->>'entry_type'
) AS held;

-- name: ClearWorkflowScheme :execrows
DELETE FROM workflow_scheme;

-- name: ImportPage :execrows
-- The search document and the references are the triggers' to build.
INSERT INTO page (
  id, tenant_id, hub_id, parent_id, title, body, icon, order_key, template, content_language,
  owner_id, verified_until, verified_at, verified_by, archived_at, deleted_at,
  created_by, created_at, updated_by, updated_at, version, restricted, daily_for
)
SELECT
  r.id, r.tenant_id, r.hub_id, r.parent_id, r.title, r.body, r.icon, r.order_key, r.template,
  r.content_language, r.owner_id, r.verified_until, r.verified_at, r.verified_by, r.archived_at,
  r.deleted_at, r.created_by, r.created_at, r.updated_by, r.updated_at, r.version,
  coalesce(r.restricted, false), r.daily_for
FROM jsonb_populate_record(
  NULL::page,
  (sqlc.arg('payload')::jsonb - 'search_document') || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  parent_id = EXCLUDED.parent_id,
  title = EXCLUDED.title,
  body = EXCLUDED.body,
  icon = EXCLUDED.icon,
  order_key = EXCLUDED.order_key,
  template = EXCLUDED.template,
  content_language = EXCLUDED.content_language,
  owner_id = EXCLUDED.owner_id,
  verified_until = EXCLUDED.verified_until,
  verified_at = EXCLUDED.verified_at,
  verified_by = EXCLUDED.verified_by,
  archived_at = EXCLUDED.archived_at,
  deleted_at = EXCLUDED.deleted_at,
  updated_by = EXCLUDED.updated_by,
  updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version,
  restricted = EXCLUDED.restricted,
  daily_for = EXCLUDED.daily_for
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsPage :one
SELECT EXISTS (SELECT 1 FROM page WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearPage :execrows
DELETE FROM page;

-- name: ImportPageVersion :execrows
INSERT INTO page_version (tenant_id, page_id, seq, title, body, author_id, name, restored, created_at)
SELECT r.tenant_id, r.page_id, r.seq, r.title, r.body, r.author_id, r.name, r.restored, r.created_at
FROM jsonb_populate_record(
  NULL::page_version,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, page_id, seq) DO UPDATE SET
  title = EXCLUDED.title,
  body = EXCLUDED.body,
  name = EXCLUDED.name,
  created_at = EXCLUDED.created_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsPageVersion :one
SELECT EXISTS (
  SELECT 1 FROM page_version
  WHERE page_id = (sqlc.arg('payload')::jsonb->>'page_id')::uuid
    AND seq = (sqlc.arg('payload')::jsonb->>'seq')::int
) AS held;

-- name: ClearPageVersion :execrows
DELETE FROM page_version;

-- name: ImportPageComment :execrows
INSERT INTO page_comment (
  id, tenant_id, page_id, parent_id, author_id, body, anchor, quote, resolved_at, resolved_by,
  created_at, edited_at, deleted_at, version
)
SELECT
  r.id, r.tenant_id, r.page_id, r.parent_id, r.author_id, r.body, r.anchor, r.quote, r.resolved_at,
  r.resolved_by, r.created_at, r.edited_at, r.deleted_at, r.version
FROM jsonb_populate_record(
  NULL::page_comment,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  body = EXCLUDED.body,
  resolved_at = EXCLUDED.resolved_at,
  resolved_by = EXCLUDED.resolved_by,
  edited_at = EXCLUDED.edited_at,
  deleted_at = EXCLUDED.deleted_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsPageComment :one
SELECT EXISTS (SELECT 1 FROM page_comment WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearPageComment :execrows
DELETE FROM page_comment;

-- name: ImportPageRestriction :execrows
INSERT INTO page_restriction (tenant_id, page_id, account_id, granted_by, granted_at)
SELECT r.tenant_id, r.page_id, r.account_id, r.granted_by, r.granted_at
FROM jsonb_populate_record(
  NULL::page_restriction,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, page_id, account_id) DO NOTHING;

-- name: HoldsPageRestriction :one
SELECT EXISTS (
  SELECT 1 FROM page_restriction
  WHERE page_id = (sqlc.arg('payload')::jsonb->>'page_id')::uuid
    AND account_id = (sqlc.arg('payload')::jsonb->>'account_id')::uuid
) AS held;

-- name: ClearPageRestriction :execrows
DELETE FROM page_restriction;

-- name: ImportPageShare :execrows
INSERT INTO page_share (tenant_id, page_id, account_id, granted_by, granted_at)
SELECT r.tenant_id, r.page_id, r.account_id, r.granted_by, r.granted_at
FROM jsonb_populate_record(
  NULL::page_share,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, page_id, account_id) DO NOTHING;

-- name: HoldsPageShare :one
SELECT EXISTS (
  SELECT 1 FROM page_share
  WHERE page_id = (sqlc.arg('payload')::jsonb->>'page_id')::uuid
    AND account_id = (sqlc.arg('payload')::jsonb->>'account_id')::uuid
) AS held;

-- name: ClearPageShare :execrows
DELETE FROM page_share;

-- name: ImportPersonalSpace :execrows
INSERT INTO personal_space (tenant_id, account_id, hub_id, created_at)
SELECT r.tenant_id, r.account_id, r.hub_id, r.created_at
FROM jsonb_populate_record(
  NULL::personal_space,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, account_id) DO UPDATE SET hub_id = EXCLUDED.hub_id, created_at = EXCLUDED.created_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsPersonalSpace :one
SELECT EXISTS (
  SELECT 1 FROM personal_space WHERE account_id = (sqlc.arg('payload')::jsonb->>'account_id')::uuid
) AS held;

-- name: ClearPersonalSpace :execrows
DELETE FROM personal_space;

-- name: ImportPagePublicLink :execrows
-- An export carries no token hash (tenant-export.md §9): such a link is skipped, and its page
-- is made a new link after the import.
INSERT INTO page_public_link (tenant_id, page_id, token_hash, created_by, created_at)
SELECT r.tenant_id, r.page_id, r.token_hash, r.created_by, r.created_at
FROM jsonb_populate_record(
  NULL::page_public_link,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
WHERE r.token_hash IS NOT NULL
ON CONFLICT (tenant_id, page_id) DO UPDATE SET
  token_hash = EXCLUDED.token_hash, created_by = EXCLUDED.created_by, created_at = EXCLUDED.created_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsPagePublicLink :one
SELECT EXISTS (
  SELECT 1 FROM page_public_link WHERE page_id = (sqlc.arg('payload')::jsonb->>'page_id')::uuid
) AS held;

-- name: ClearPagePublicLink :execrows
DELETE FROM page_public_link;

-- name: ImportPageArticle :execrows
INSERT INTO page_article (tenant_id, page_id, collection_id, published_by, published_at, views)
SELECT r.tenant_id, r.page_id, r.collection_id, r.published_by, r.published_at, coalesce(r.views, 0)
FROM jsonb_populate_record(
  NULL::page_article,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, page_id, collection_id) DO UPDATE SET
  published_by = EXCLUDED.published_by, published_at = EXCLUDED.published_at, views = EXCLUDED.views
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsPageArticle :one
SELECT EXISTS (
  SELECT 1 FROM page_article
  WHERE page_id = (sqlc.arg('payload')::jsonb->>'page_id')::uuid
    AND collection_id = (sqlc.arg('payload')::jsonb->>'collection_id')::uuid
) AS held;

-- name: ClearPageArticle :execrows
DELETE FROM page_article;

-- name: ImportPageArticleRating :execrows
INSERT INTO page_article_rating (tenant_id, page_id, collection_id, account_id, helpful, rated_at)
SELECT r.tenant_id, r.page_id, r.collection_id, r.account_id, r.helpful, r.rated_at
FROM jsonb_populate_record(
  NULL::page_article_rating,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, page_id, collection_id, account_id) DO UPDATE SET
  helpful = EXCLUDED.helpful, rated_at = EXCLUDED.rated_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsPageArticleRating :one
SELECT EXISTS (
  SELECT 1 FROM page_article_rating
  WHERE page_id = (sqlc.arg('payload')::jsonb->>'page_id')::uuid
    AND collection_id = (sqlc.arg('payload')::jsonb->>'collection_id')::uuid
    AND account_id = (sqlc.arg('payload')::jsonb->>'account_id')::uuid
) AS held;

-- name: ClearPageArticleRating :execrows
DELETE FROM page_article_rating;

-- name: ImportRealtimeDocument :execrows
-- A live document's head, log and snapshots (migration 0230): written once each, so an overwrite
-- has only the head's places to bring back.
INSERT INTO realtime_document (tenant_id, kind, document_id, head_seq, snapshot_seq, created_at, updated_at)
SELECT r.tenant_id, r.kind, r.document_id, r.head_seq, r.snapshot_seq, r.created_at, r.updated_at
FROM jsonb_populate_record(
  NULL::realtime_document,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, kind, document_id) DO UPDATE SET
  head_seq = EXCLUDED.head_seq,
  snapshot_seq = EXCLUDED.snapshot_seq,
  updated_at = EXCLUDED.updated_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsRealtimeDocument :one
SELECT EXISTS (
  SELECT 1 FROM realtime_document
  WHERE kind = sqlc.arg('payload')::jsonb->>'kind'
    AND document_id = (sqlc.arg('payload')::jsonb->>'document_id')::uuid
) AS held;

-- name: ClearRealtimeDocument :execrows
DELETE FROM realtime_document;

-- name: ImportRealtimeUpdate :execrows
INSERT INTO realtime_update (tenant_id, kind, document_id, seq, data, author_id, created_at, compacted_at)
SELECT r.tenant_id, r.kind, r.document_id, r.seq, r.data, r.author_id, r.created_at, r.compacted_at
FROM jsonb_populate_record(
  NULL::realtime_update,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, kind, document_id, seq) DO UPDATE SET
  compacted_at = EXCLUDED.compacted_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsRealtimeUpdate :one
SELECT EXISTS (
  SELECT 1 FROM realtime_update
  WHERE kind = sqlc.arg('payload')::jsonb->>'kind'
    AND document_id = (sqlc.arg('payload')::jsonb->>'document_id')::uuid
    AND seq = (sqlc.arg('payload')::jsonb->>'seq')::bigint
) AS held;

-- name: ClearRealtimeUpdate :execrows
DELETE FROM realtime_update;

-- name: ImportRealtimeSnapshot :execrows
INSERT INTO realtime_snapshot (tenant_id, kind, document_id, up_to_seq, state, created_at, superseded_at)
SELECT r.tenant_id, r.kind, r.document_id, r.up_to_seq, r.state, r.created_at, r.superseded_at
FROM jsonb_populate_record(
  NULL::realtime_snapshot,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, kind, document_id, up_to_seq) DO UPDATE SET
  superseded_at = EXCLUDED.superseded_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsRealtimeSnapshot :one
SELECT EXISTS (
  SELECT 1 FROM realtime_snapshot
  WHERE kind = sqlc.arg('payload')::jsonb->>'kind'
    AND document_id = (sqlc.arg('payload')::jsonb->>'document_id')::uuid
    AND up_to_seq = (sqlc.arg('payload')::jsonb->>'up_to_seq')::bigint
) AS held;

-- name: ClearRealtimeSnapshot :execrows
DELETE FROM realtime_snapshot;

-- name: ImportWhiteboard :execrows
-- Where the board points is the trigger's to build.
INSERT INTO whiteboard (id, tenant_id, hub_id, title, scene, deleted_at, created_by, created_at, updated_by, updated_at, version)
SELECT r.id, r.tenant_id, r.hub_id, r.title, r.scene, r.deleted_at, r.created_by, r.created_at, r.updated_by,
       r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::whiteboard,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  title = EXCLUDED.title,
  scene = EXCLUDED.scene,
  deleted_at = EXCLUDED.deleted_at,
  updated_by = EXCLUDED.updated_by,
  updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsWhiteboard :one
SELECT EXISTS (SELECT 1 FROM whiteboard WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearWhiteboard :execrows
DELETE FROM whiteboard;

-- name: ImportWhiteboardVersion :execrows
INSERT INTO whiteboard_version (tenant_id, whiteboard_id, seq, title, scene, author_id, name, created_at)
SELECT r.tenant_id, r.whiteboard_id, r.seq, r.title, r.scene, r.author_id, r.name, r.created_at
FROM jsonb_populate_record(
  NULL::whiteboard_version,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, whiteboard_id, seq) DO UPDATE SET
  title = EXCLUDED.title,
  scene = EXCLUDED.scene,
  name = EXCLUDED.name,
  created_at = EXCLUDED.created_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsWhiteboardVersion :one
SELECT EXISTS (
  SELECT 1 FROM whiteboard_version
  WHERE whiteboard_id = (sqlc.arg('payload')::jsonb->>'whiteboard_id')::uuid
    AND seq = (sqlc.arg('payload')::jsonb->>'seq')::int
) AS held;

-- name: ClearWhiteboardVersion :execrows
DELETE FROM whiteboard_version;

-- name: ImportWhiteboardMedia :execrows
INSERT INTO whiteboard_media (tenant_id, whiteboard_id, media_id)
SELECT r.tenant_id, r.whiteboard_id, r.media_id
FROM jsonb_populate_record(
  NULL::whiteboard_media,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, whiteboard_id, media_id) DO NOTHING;

-- name: HoldsWhiteboardMedia :one
SELECT EXISTS (
  SELECT 1 FROM whiteboard_media
  WHERE whiteboard_id = (sqlc.arg('payload')::jsonb->>'whiteboard_id')::uuid
    AND media_id = (sqlc.arg('payload')::jsonb->>'media_id')::uuid
) AS held;

-- name: ClearWhiteboardMedia :execrows
DELETE FROM whiteboard_media;


-- name: ImportCmdbClass :execrows
INSERT INTO cmdb_class (id, tenant_id, key, parent_key, name, icon, abstract, attributes, rules, required, staleness_days, archived_at, created_at, updated_at, version)
SELECT r.id, r.tenant_id, r.key, r.parent_key, r.name, r.icon, r.abstract, r.attributes, r.rules, r.required, r.staleness_days, r.archived_at, r.created_at, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::cmdb_class,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  parent_key = EXCLUDED.parent_key,
  name = EXCLUDED.name,
  icon = EXCLUDED.icon,
  abstract = EXCLUDED.abstract,
  attributes = EXCLUDED.attributes,
  rules = EXCLUDED.rules,
  required = EXCLUDED.required,
  staleness_days = EXCLUDED.staleness_days,
  archived_at = EXCLUDED.archived_at,
  updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCmdbClass :one
SELECT EXISTS (SELECT 1 FROM cmdb_class WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCmdbClass :execrows
DELETE FROM cmdb_class;

-- name: ImportCmdbRelationType :execrows
INSERT INTO cmdb_relation_type (id, tenant_id, key, name, reverse_name, impact, pairs, archived_at, created_at, updated_at, version)
SELECT r.id, r.tenant_id, r.key, r.name, r.reverse_name, r.impact, r.pairs, r.archived_at, r.created_at, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::cmdb_relation_type,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  reverse_name = EXCLUDED.reverse_name,
  impact = EXCLUDED.impact,
  pairs = EXCLUDED.pairs,
  archived_at = EXCLUDED.archived_at,
  updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCmdbRelationType :one
SELECT EXISTS (SELECT 1 FROM cmdb_relation_type WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCmdbRelationType :execrows
DELETE FROM cmdb_relation_type;

-- name: ImportCmdbSource :execrows
INSERT INTO cmdb_source (id, tenant_id, key, kind, rank, classes, may_create, mapping, last_run_at, created_at, updated_at, version)
SELECT r.id, r.tenant_id, r.key, r.kind, r.rank, r.classes, r.may_create, r.mapping, r.last_run_at, r.created_at, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::cmdb_source,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  kind = EXCLUDED.kind,
  rank = EXCLUDED.rank,
  classes = EXCLUDED.classes,
  may_create = EXCLUDED.may_create,
  mapping = EXCLUDED.mapping,
  last_run_at = EXCLUDED.last_run_at,
  updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCmdbSource :one
SELECT EXISTS (SELECT 1 FROM cmdb_source WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCmdbSource :execrows
DELETE FROM cmdb_source;

-- name: ImportCmdbCi :execrows
INSERT INTO cmdb_ci (id, tenant_id, hub_id, class_key, name, stage, status, attributes, provenance, owner_account_id, owner_group_id, location_ci_id, protection, review_due, quantity, last_seen_at, merged_into, created_by, created_at, updated_by, updated_at, version)
SELECT r.id, r.tenant_id, r.hub_id, r.class_key, r.name, r.stage, r.status, r.attributes, r.provenance, r.owner_account_id, r.owner_group_id, r.location_ci_id, r.protection, r.review_due, r.quantity, r.last_seen_at, r.merged_into, r.created_by, r.created_at, r.updated_by, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::cmdb_ci,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  class_key = EXCLUDED.class_key,
  name = EXCLUDED.name,
  stage = EXCLUDED.stage,
  status = EXCLUDED.status,
  attributes = EXCLUDED.attributes,
  provenance = EXCLUDED.provenance,
  owner_account_id = EXCLUDED.owner_account_id,
  owner_group_id = EXCLUDED.owner_group_id,
  location_ci_id = EXCLUDED.location_ci_id,
  protection = EXCLUDED.protection,
  review_due = EXCLUDED.review_due,
  quantity = EXCLUDED.quantity,
  last_seen_at = EXCLUDED.last_seen_at,
  merged_into = EXCLUDED.merged_into,
  updated_by = EXCLUDED.updated_by,
  updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCmdbCi :one
SELECT EXISTS (SELECT 1 FROM cmdb_ci WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCmdbCi :execrows
DELETE FROM cmdb_ci;

-- name: ImportCmdbCiIdentifier :execrows
INSERT INTO cmdb_ci_identifier (tenant_id, ci_id, hub_id, name, value)
SELECT r.tenant_id, r.ci_id, r.hub_id, r.name, r.value
FROM jsonb_populate_record(
  NULL::cmdb_ci_identifier,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, ci_id, name) DO UPDATE SET
  value = EXCLUDED.value
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCmdbCiIdentifier :one
SELECT EXISTS (SELECT 1 FROM cmdb_ci_identifier WHERE ci_id = (sqlc.arg('payload')::jsonb->>'ci_id')::uuid AND name = (sqlc.arg('payload')::jsonb->>'name')) AS held;

-- name: ClearCmdbCiIdentifier :execrows
DELETE FROM cmdb_ci_identifier;

-- name: ImportCmdbRelation :execrows
INSERT INTO cmdb_relation (id, tenant_id, hub_id, type_key, source_ci_id, target_ci_id, data_source, first_seen, last_seen, created_by)
SELECT r.id, r.tenant_id, r.hub_id, r.type_key, r.source_ci_id, r.target_ci_id, r.data_source, r.first_seen, r.last_seen, r.created_by
FROM jsonb_populate_record(
  NULL::cmdb_relation,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  data_source = EXCLUDED.data_source,
  last_seen = EXCLUDED.last_seen
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCmdbRelation :one
SELECT EXISTS (SELECT 1 FROM cmdb_relation WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCmdbRelation :execrows
DELETE FROM cmdb_relation;

-- name: ImportCmdbRun :execrows
INSERT INTO cmdb_run (id, tenant_id, hub_id, source_key, actor_id, started_at, counts, decisions)
SELECT r.id, r.tenant_id, r.hub_id, r.source_key, r.actor_id, r.started_at, r.counts, r.decisions
FROM jsonb_populate_record(
  NULL::cmdb_run,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  counts = EXCLUDED.counts,
  decisions = EXCLUDED.decisions
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCmdbRun :one
SELECT EXISTS (SELECT 1 FROM cmdb_run WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCmdbRun :execrows
DELETE FROM cmdb_run;

-- name: ImportCmdbHold :execrows
INSERT INTO cmdb_hold (id, tenant_id, hub_id, run_id, source_key, item, reason, created_at, resolved_at, resolved_by)
SELECT r.id, r.tenant_id, r.hub_id, r.run_id, r.source_key, r.item, r.reason, r.created_at, r.resolved_at, r.resolved_by
FROM jsonb_populate_record(
  NULL::cmdb_hold,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  resolved_at = EXCLUDED.resolved_at,
  resolved_by = EXCLUDED.resolved_by
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCmdbHold :one
SELECT EXISTS (SELECT 1 FROM cmdb_hold WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCmdbHold :execrows
DELETE FROM cmdb_hold;

-- name: ImportCmdbDuplicate :execrows
INSERT INTO cmdb_duplicate (id, tenant_id, hub_id, ci_ids, rule, created_at, resolved_at, resolved_by, survivor_id)
SELECT r.id, r.tenant_id, r.hub_id, r.ci_ids, r.rule, r.created_at, r.resolved_at, r.resolved_by, r.survivor_id
FROM jsonb_populate_record(
  NULL::cmdb_duplicate,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  resolved_at = EXCLUDED.resolved_at,
  resolved_by = EXCLUDED.resolved_by,
  survivor_id = EXCLUDED.survivor_id
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCmdbDuplicate :one
SELECT EXISTS (SELECT 1 FROM cmdb_duplicate WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCmdbDuplicate :execrows
DELETE FROM cmdb_duplicate;

-- name: ImportCmdbConnector :execrows
INSERT INTO cmdb_connector (id, tenant_id, hub_id, key, kind, source_key, base_url, settings, allowed_hosts,
  every_minutes, enabled, collection_id, run_as, credential_enc, credential_key_id, cursor, next_run_at,
  last_run_at, last_run_id, last_outcome, last_error, failures, created_at, updated_at, version)
SELECT r.id, r.tenant_id, r.hub_id, r.key, r.kind, r.source_key, r.base_url, r.settings, r.allowed_hosts,
  r.every_minutes, r.enabled, r.collection_id, r.run_as, r.credential_enc, r.credential_key_id, r.cursor, r.next_run_at,
  r.last_run_at, r.last_run_id, r.last_outcome, r.last_error, r.failures, r.created_at, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::cmdb_connector,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  source_key = EXCLUDED.source_key,
  base_url = EXCLUDED.base_url,
  settings = EXCLUDED.settings,
  allowed_hosts = EXCLUDED.allowed_hosts,
  every_minutes = EXCLUDED.every_minutes,
  enabled = EXCLUDED.enabled,
  collection_id = EXCLUDED.collection_id,
  run_as = EXCLUDED.run_as,
  credential_enc = EXCLUDED.credential_enc,
  credential_key_id = EXCLUDED.credential_key_id,
  cursor = EXCLUDED.cursor,
  next_run_at = EXCLUDED.next_run_at,
  last_run_at = EXCLUDED.last_run_at,
  last_run_id = EXCLUDED.last_run_id,
  last_outcome = EXCLUDED.last_outcome,
  last_error = EXCLUDED.last_error,
  failures = EXCLUDED.failures,
  updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCmdbConnector :one
SELECT EXISTS (SELECT 1 FROM cmdb_connector WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCmdbConnector :execrows
DELETE FROM cmdb_connector;

-- name: ImportCmdbAlert :execrows
INSERT INTO cmdb_alert (id, tenant_id, hub_id, connector_id, dedup_key, status, severity, title, ci_id, entry_id,
  first_at, last_at, resolved_at, count)
SELECT r.id, r.tenant_id, r.hub_id, r.connector_id, r.dedup_key, r.status, r.severity, r.title, r.ci_id, r.entry_id,
  r.first_at, r.last_at, r.resolved_at, r.count
FROM jsonb_populate_record(
  NULL::cmdb_alert,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  status = EXCLUDED.status,
  severity = EXCLUDED.severity,
  title = EXCLUDED.title,
  ci_id = EXCLUDED.ci_id,
  entry_id = EXCLUDED.entry_id,
  last_at = EXCLUDED.last_at,
  resolved_at = EXCLUDED.resolved_at,
  count = EXCLUDED.count
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCmdbAlert :one
SELECT EXISTS (SELECT 1 FROM cmdb_alert WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCmdbAlert :execrows
DELETE FROM cmdb_alert;

-- name: ImportBusinessCalendar :execrows
INSERT INTO business_calendar (id, tenant_id, key, name, spec, created_at, updated_at, version)
SELECT r.id, r.tenant_id, r.key, r.name, r.spec, r.created_at, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::business_calendar,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name, spec = EXCLUDED.spec, updated_at = EXCLUDED.updated_at, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsBusinessCalendar :one
SELECT EXISTS (SELECT 1 FROM business_calendar WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearBusinessCalendar :execrows
DELETE FROM business_calendar;

-- name: ImportHubCalendar :execrows
INSERT INTO hub_business_calendar (id, tenant_id, hub_id, key, name, spec, created_at, updated_at, version)
SELECT r.id, r.tenant_id, r.hub_id, r.key, r.name, r.spec, r.created_at, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::hub_business_calendar,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name, spec = EXCLUDED.spec, updated_at = EXCLUDED.updated_at, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsHubCalendar :one
SELECT EXISTS (SELECT 1 FROM hub_business_calendar WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearHubCalendar :execrows
DELETE FROM hub_business_calendar;

-- name: ImportSlaPolicy :execrows
INSERT INTO sla_policy (id, tenant_id, key, name, calendar_key, entry_types, priority_field, goals, position, enabled, created_at, updated_at, version)
SELECT r.id, r.tenant_id, r.key, r.name, r.calendar_key, r.entry_types, r.priority_field, r.goals, r.position, r.enabled, r.created_at, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::sla_policy,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name, calendar_key = EXCLUDED.calendar_key, entry_types = EXCLUDED.entry_types,
  priority_field = EXCLUDED.priority_field, goals = EXCLUDED.goals, position = EXCLUDED.position,
  enabled = EXCLUDED.enabled, updated_at = EXCLUDED.updated_at, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsSlaPolicy :one
SELECT EXISTS (SELECT 1 FROM sla_policy WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearSlaPolicy :execrows
DELETE FROM sla_policy;

-- name: ImportEntrySla :execrows
INSERT INTO entry_sla (id, tenant_id, item_id, goal, policy_key, target_seconds, standing, armed_at, due_at, at_risk_at, met_at, breached_at, updated_at, version)
SELECT r.id, r.tenant_id, r.item_id, r.goal, r.policy_key, r.target_seconds, r.standing, r.armed_at, r.due_at, r.at_risk_at, r.met_at, r.breached_at, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::entry_sla,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  policy_key = EXCLUDED.policy_key, target_seconds = EXCLUDED.target_seconds, standing = EXCLUDED.standing,
  armed_at = EXCLUDED.armed_at, due_at = EXCLUDED.due_at, at_risk_at = EXCLUDED.at_risk_at,
  met_at = EXCLUDED.met_at, breached_at = EXCLUDED.breached_at, updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsEntrySla :one
SELECT EXISTS (SELECT 1 FROM entry_sla WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearEntrySla :execrows
DELETE FROM entry_sla;

-- name: ImportReplyTemplate :execrows
INSERT INTO reply_template (id, tenant_id, key, name, body, created_at, updated_at, version)
SELECT r.id, r.tenant_id, r.key, r.name, r.body, r.created_at, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::reply_template,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name, body = EXCLUDED.body, updated_at = EXCLUDED.updated_at, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsReplyTemplate :one
SELECT EXISTS (SELECT 1 FROM reply_template WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearReplyTemplate :execrows
DELETE FROM reply_template;

-- name: ImportSatisfactionRating :execrows
INSERT INTO satisfaction_rating (id, tenant_id, item_id, score, rated_at, version)
SELECT r.id, r.tenant_id, r.item_id, r.score, r.rated_at, r.version
FROM jsonb_populate_record(
  NULL::satisfaction_rating,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  score = EXCLUDED.score, rated_at = EXCLUDED.rated_at, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsSatisfactionRating :one
SELECT EXISTS (SELECT 1 FROM satisfaction_rating WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearSatisfactionRating :execrows
DELETE FROM satisfaction_rating;

-- name: ImportEntrySchedule :execrows
INSERT INTO entry_schedule (tenant_id, entry_id, mode, milestone, updated_at, version)
SELECT r.tenant_id, r.entry_id, r.mode, r.milestone, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::entry_schedule,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, entry_id) DO UPDATE SET
  mode = EXCLUDED.mode, milestone = EXCLUDED.milestone, updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsEntrySchedule :one
SELECT EXISTS (
  SELECT 1 FROM entry_schedule WHERE entry_id = (sqlc.arg('payload')::jsonb->>'entry_id')::uuid
) AS held;

-- name: ClearEntrySchedule :execrows
DELETE FROM entry_schedule;

-- name: ImportPlanBaseline :execrows
-- A baseline is taken and deleted, never changed: an overwrite has nothing to replace.
INSERT INTO plan_baseline (id, tenant_id, plan_id, name, taken_at, taken_by, entry_count)
SELECT r.id, r.tenant_id, r.plan_id, r.name, r.taken_at, r.taken_by, r.entry_count
FROM jsonb_populate_record(
  NULL::plan_baseline,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsPlanBaseline :one
SELECT EXISTS (SELECT 1 FROM plan_baseline WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearPlanBaseline :execrows
DELETE FROM plan_baseline;

-- name: ImportPlanBaselineEntry :execrows
INSERT INTO plan_baseline_entry (tenant_id, baseline_id, entry_id, start_at, due_at)
SELECT r.tenant_id, r.baseline_id, r.entry_id, r.start_at, r.due_at
FROM jsonb_populate_record(
  NULL::plan_baseline_entry,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, baseline_id, entry_id) DO UPDATE SET
  start_at = EXCLUDED.start_at, due_at = EXCLUDED.due_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsPlanBaselineEntry :one
SELECT EXISTS (
  SELECT 1 FROM plan_baseline_entry
  WHERE baseline_id = (sqlc.arg('payload')::jsonb->>'baseline_id')::uuid
    AND entry_id = (sqlc.arg('payload')::jsonb->>'entry_id')::uuid
) AS held;

-- name: ClearPlanBaselineEntry :execrows
DELETE FROM plan_baseline_entry;

-- name: ImportSprint :execrows
INSERT INTO sprint (id, tenant_id, collection_id, name, goal, start_on, end_on, state, unit,
  committed_value, committed_count, completed_value, completed_count, started_at, closed_at,
  created_at, created_by, updated_at, version)
SELECT r.id, r.tenant_id, r.collection_id, r.name, r.goal, r.start_on, r.end_on, r.state, r.unit,
  r.committed_value, r.committed_count, r.completed_value, r.completed_count, r.started_at,
  r.closed_at, r.created_at, r.created_by, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::sprint,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name, goal = EXCLUDED.goal, start_on = EXCLUDED.start_on, end_on = EXCLUDED.end_on,
  state = EXCLUDED.state, unit = EXCLUDED.unit, committed_value = EXCLUDED.committed_value,
  committed_count = EXCLUDED.committed_count, completed_value = EXCLUDED.completed_value,
  completed_count = EXCLUDED.completed_count, started_at = EXCLUDED.started_at,
  closed_at = EXCLUDED.closed_at, updated_at = EXCLUDED.updated_at, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsSprint :one
SELECT EXISTS (SELECT 1 FROM sprint WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearSprint :execrows
DELETE FROM sprint;

-- name: ImportSprintEntry :execrows
INSERT INTO sprint_entry (tenant_id, sprint_id, entry_id, added_at, removed_at, committed, outcome)
SELECT r.tenant_id, r.sprint_id, r.entry_id, r.added_at, r.removed_at, r.committed, r.outcome
FROM jsonb_populate_record(
  NULL::sprint_entry,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, sprint_id, entry_id) DO UPDATE SET
  added_at = EXCLUDED.added_at, removed_at = EXCLUDED.removed_at, committed = EXCLUDED.committed,
  outcome = EXCLUDED.outcome
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsSprintEntry :one
SELECT EXISTS (
  SELECT 1 FROM sprint_entry
  WHERE sprint_id = (sqlc.arg('payload')::jsonb->>'sprint_id')::uuid
    AND entry_id = (sqlc.arg('payload')::jsonb->>'entry_id')::uuid
) AS held;

-- name: ClearSprintEntry :execrows
DELETE FROM sprint_entry;

-- name: ImportEntryEstimate :execrows
INSERT INTO entry_estimate (tenant_id, entry_id, unit, value, size, updated_at, updated_by, version)
SELECT r.tenant_id, r.entry_id, r.unit, r.value, r.size, r.updated_at, r.updated_by, r.version
FROM jsonb_populate_record(
  NULL::entry_estimate,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, entry_id) DO UPDATE SET
  unit = EXCLUDED.unit, value = EXCLUDED.value, size = EXCLUDED.size,
  updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsEntryEstimate :one
SELECT EXISTS (
  SELECT 1 FROM entry_estimate WHERE entry_id = (sqlc.arg('payload')::jsonb->>'entry_id')::uuid
) AS held;

-- name: ClearEntryEstimate :execrows
DELETE FROM entry_estimate;

-- name: ImportTimeWorkType :execrows
INSERT INTO time_work_type (id, tenant_id, key, name, billable, archived, position, created_at, updated_at, version)
SELECT r.id, r.tenant_id, r.key, r.name, r.billable, r.archived, r.position, r.created_at, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::time_work_type,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name, billable = EXCLUDED.billable, archived = EXCLUDED.archived, position = EXCLUDED.position, updated_at = EXCLUDED.updated_at, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsTimeWorkType :one
SELECT EXISTS (SELECT 1 FROM time_work_type WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearTimeWorkType :execrows
DELETE FROM time_work_type;

-- name: ImportTimeRecord :execrows
INSERT INTO time_record (id, tenant_id, account_id, entry_id, work_type, day, started_at, minutes, note, billable, source, corrects, created_at, updated_at, version)
SELECT r.id, r.tenant_id, r.account_id, r.entry_id, r.work_type, r.day, r.started_at, r.minutes, r.note, r.billable, r.source, r.corrects, r.created_at, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::time_record,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  entry_id = EXCLUDED.entry_id, work_type = EXCLUDED.work_type, day = EXCLUDED.day, started_at = EXCLUDED.started_at, minutes = EXCLUDED.minutes, note = EXCLUDED.note, billable = EXCLUDED.billable, updated_at = EXCLUDED.updated_at, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsTimeRecord :one
SELECT EXISTS (SELECT 1 FROM time_record WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearTimeRecord :execrows
DELETE FROM time_record;

-- name: ImportTimeTimer :execrows
INSERT INTO time_timer (id, tenant_id, account_id, entry_id, work_type, note, billable, started_at, version)
SELECT r.id, r.tenant_id, r.account_id, r.entry_id, r.work_type, r.note, r.billable, r.started_at, r.version
FROM jsonb_populate_record(
  NULL::time_timer,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  entry_id = EXCLUDED.entry_id, work_type = EXCLUDED.work_type, note = EXCLUDED.note, billable = EXCLUDED.billable, started_at = EXCLUDED.started_at, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsTimeTimer :one
SELECT EXISTS (SELECT 1 FROM time_timer WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearTimeTimer :execrows
DELETE FROM time_timer;

-- name: ImportTimesheet :execrows
INSERT INTO timesheet (id, tenant_id, account_id, week_start, state, minutes, submitted_at, decided_at, decided_by, reason, updated_at, version)
SELECT r.id, r.tenant_id, r.account_id, r.week_start, r.state, r.minutes, r.submitted_at, r.decided_at, r.decided_by, r.reason, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::timesheet,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  state = EXCLUDED.state, minutes = EXCLUDED.minutes, submitted_at = EXCLUDED.submitted_at, decided_at = EXCLUDED.decided_at, decided_by = EXCLUDED.decided_by, reason = EXCLUDED.reason, updated_at = EXCLUDED.updated_at, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsTimesheet :one
SELECT EXISTS (SELECT 1 FROM timesheet WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearTimesheet :execrows
DELETE FROM timesheet;

-- name: ImportTimePolicy :execrows
INSERT INTO time_policy (id, tenant_id, locked_through, visibility, updated_at, updated_by, version)
SELECT r.id, r.tenant_id, r.locked_through, r.visibility, r.updated_at, r.updated_by, r.version
FROM jsonb_populate_record(
  NULL::time_policy,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  locked_through = EXCLUDED.locked_through, visibility = EXCLUDED.visibility, updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsTimePolicy :one
SELECT EXISTS (SELECT 1 FROM time_policy WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearTimePolicy :execrows
DELETE FROM time_policy;

-- name: ImportTimeCapacity :execrows
INSERT INTO time_capacity (id, tenant_id, account_id, from_day, week_minutes, updated_at, updated_by, version)
SELECT r.id, r.tenant_id, r.account_id, r.from_day, r.week_minutes, r.updated_at, r.updated_by, r.version
FROM jsonb_populate_record(
  NULL::time_capacity,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  week_minutes = EXCLUDED.week_minutes, updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsTimeCapacity :one
SELECT EXISTS (SELECT 1 FROM time_capacity WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearTimeCapacity :execrows
DELETE FROM time_capacity;

-- name: ImportTimeAbsence :execrows
INSERT INTO time_absence (id, tenant_id, account_id, from_day, to_day, kind, half, created_at, created_by, updated_at, version)
SELECT r.id, r.tenant_id, r.account_id, r.from_day, r.to_day, r.kind, r.half, r.created_at, r.created_by, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::time_absence,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  from_day = EXCLUDED.from_day, to_day = EXCLUDED.to_day, kind = EXCLUDED.kind, half = EXCLUDED.half, updated_at = EXCLUDED.updated_at, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsTimeAbsence :one
SELECT EXISTS (SELECT 1 FROM time_absence WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearTimeAbsence :execrows
DELETE FROM time_absence;

-- name: ImportTimeCostRate :execrows
INSERT INTO time_cost_rate (id, tenant_id, account_id, role, hour_minor, currency, from_day, to_day, updated_at, updated_by, version)
SELECT r.id, r.tenant_id, r.account_id, r.role, r.hour_minor, r.currency, r.from_day, r.to_day, r.updated_at, r.updated_by, r.version
FROM jsonb_populate_record(
  NULL::time_cost_rate,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  hour_minor = EXCLUDED.hour_minor, currency = EXCLUDED.currency, from_day = EXCLUDED.from_day, to_day = EXCLUDED.to_day, updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsTimeCostRate :one
SELECT EXISTS (SELECT 1 FROM time_cost_rate WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearTimeCostRate :execrows
DELETE FROM time_cost_rate;

-- name: ImportTimeBudget :execrows
INSERT INTO time_budget (id, tenant_id, plan_id, minutes, amount_minor, currency, updated_at, updated_by, version)
SELECT r.id, r.tenant_id, r.plan_id, r.minutes, r.amount_minor, r.currency, r.updated_at, r.updated_by, r.version
FROM jsonb_populate_record(
  NULL::time_budget,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  minutes = EXCLUDED.minutes, amount_minor = EXCLUDED.amount_minor, currency = EXCLUDED.currency, updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsTimeBudget :one
SELECT EXISTS (SELECT 1 FROM time_budget WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearTimeBudget :execrows
DELETE FROM time_budget;

-- name: ImportGoalPeriod :execrows
INSERT INTO goal_period (id, tenant_id, name, start_on, end_on, state, closed_at, closed_by, created_at, created_by, updated_at, version)
SELECT r.id, r.tenant_id, r.name, r.start_on, r.end_on, r.state, r.closed_at, r.closed_by, r.created_at, r.created_by, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::goal_period,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name, start_on = EXCLUDED.start_on, end_on = EXCLUDED.end_on, state = EXCLUDED.state, closed_at = EXCLUDED.closed_at, closed_by = EXCLUDED.closed_by, created_at = EXCLUDED.created_at, created_by = EXCLUDED.created_by, updated_at = EXCLUDED.updated_at, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsGoalPeriod :one
SELECT EXISTS (SELECT 1 FROM goal_period WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearGoalPeriod :execrows
DELETE FROM goal_period;

-- name: ImportGoal :execrows
INSERT INTO goal (id, tenant_id, period_id, hub_id, parent_id, title, description, owner_id, status, confidence, key_results, grade, created_at, created_by, updated_at, updated_by, version)
SELECT r.id, r.tenant_id, r.period_id, r.hub_id, r.parent_id, r.title, r.description, r.owner_id, r.status, r.confidence, r.key_results, r.grade, r.created_at, r.created_by, r.updated_at, r.updated_by, r.version
FROM jsonb_populate_record(
  NULL::goal,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  period_id = EXCLUDED.period_id, hub_id = EXCLUDED.hub_id, parent_id = EXCLUDED.parent_id, title = EXCLUDED.title, description = EXCLUDED.description, owner_id = EXCLUDED.owner_id, status = EXCLUDED.status, confidence = EXCLUDED.confidence, key_results = EXCLUDED.key_results, grade = EXCLUDED.grade, created_at = EXCLUDED.created_at, created_by = EXCLUDED.created_by, updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsGoal :one
SELECT EXISTS (SELECT 1 FROM goal WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearGoal :execrows
DELETE FROM goal;

-- name: ImportGoalEntry :execrows
INSERT INTO goal_entry (tenant_id, goal_id, entry_id, created_at, created_by)
SELECT r.tenant_id, r.goal_id, r.entry_id, r.created_at, r.created_by
FROM jsonb_populate_record(
  NULL::goal_entry,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, goal_id, entry_id) DO UPDATE SET
  created_at = EXCLUDED.created_at, created_by = EXCLUDED.created_by
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsGoalEntry :one
SELECT EXISTS (
  SELECT 1 FROM goal_entry
  WHERE goal_id = (sqlc.arg('payload')::jsonb->>'goal_id')::uuid
    AND entry_id = (sqlc.arg('payload')::jsonb->>'entry_id')::uuid
) AS held;

-- name: ClearGoalEntry :execrows
DELETE FROM goal_entry;

-- name: ImportGoalCheckIn :execrows
INSERT INTO goal_check_in (id, tenant_id, goal_id, author_id, status, confidence, note, "values", created_at)
SELECT r.id, r.tenant_id, r.goal_id, r.author_id, r.status, r.confidence, r.note, r."values", r.created_at
FROM jsonb_populate_record(
  NULL::goal_check_in,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  goal_id = EXCLUDED.goal_id, author_id = EXCLUDED.author_id, status = EXCLUDED.status, confidence = EXCLUDED.confidence, note = EXCLUDED.note, "values" = EXCLUDED."values", created_at = EXCLUDED.created_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsGoalCheckIn :one
SELECT EXISTS (SELECT 1 FROM goal_check_in WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearGoalCheckIn :execrows
DELETE FROM goal_check_in;

-- name: ImportPortfolio :execrows
INSERT INTO portfolio (id, tenant_id, parent_id, name, description, owner_id, status, created_at, created_by, updated_at, updated_by, version)
SELECT r.id, r.tenant_id, r.parent_id, r.name, r.description, r.owner_id, r.status, r.created_at, r.created_by, r.updated_at, r.updated_by, r.version
FROM jsonb_populate_record(
  NULL::portfolio,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  parent_id = EXCLUDED.parent_id, name = EXCLUDED.name, description = EXCLUDED.description, owner_id = EXCLUDED.owner_id, status = EXCLUDED.status, created_at = EXCLUDED.created_at, created_by = EXCLUDED.created_by, updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsPortfolio :one
SELECT EXISTS (SELECT 1 FROM portfolio WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearPortfolio :execrows
DELETE FROM portfolio;

-- name: ImportPortfolioItem :execrows
INSERT INTO portfolio_item (tenant_id, portfolio_id, container_id, position, start_on, end_on, status)
SELECT r.tenant_id, r.portfolio_id, r.container_id, r.position, r.start_on, r.end_on, r.status
FROM jsonb_populate_record(
  NULL::portfolio_item,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, portfolio_id, container_id) DO UPDATE SET
  position = EXCLUDED.position, start_on = EXCLUDED.start_on, end_on = EXCLUDED.end_on, status = EXCLUDED.status
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsPortfolioItem :one
SELECT EXISTS (
  SELECT 1 FROM portfolio_item
  WHERE portfolio_id = (sqlc.arg('payload')::jsonb->>'portfolio_id')::uuid
    AND container_id = (sqlc.arg('payload')::jsonb->>'container_id')::uuid
) AS held;

-- name: ClearPortfolioItem :execrows
DELETE FROM portfolio_item;

-- name: ImportPortfolioUpdate :execrows
INSERT INTO portfolio_update (id, tenant_id, portfolio_id, author_id, status, summary, created_at)
SELECT r.id, r.tenant_id, r.portfolio_id, r.author_id, r.status, r.summary, r.created_at
FROM jsonb_populate_record(
  NULL::portfolio_update,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  portfolio_id = EXCLUDED.portfolio_id, author_id = EXCLUDED.author_id, status = EXCLUDED.status, summary = EXCLUDED.summary, created_at = EXCLUDED.created_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsPortfolioUpdate :one
SELECT EXISTS (SELECT 1 FROM portfolio_update WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearPortfolioUpdate :execrows
DELETE FROM portfolio_update;

-- name: ImportDashboard :execrows
INSERT INTO dashboard (id, tenant_id, hub_id, owner_id, name, sharing, group_ids, widgets, created_at, updated_at, updated_by, version)
SELECT r.id, r.tenant_id, r.hub_id, r.owner_id, r.name, r.sharing, coalesce(r.group_ids, '{}'), r.widgets, r.created_at, r.updated_at, r.updated_by, r.version
FROM jsonb_populate_record(
  NULL::dashboard,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  hub_id = EXCLUDED.hub_id, owner_id = EXCLUDED.owner_id, name = EXCLUDED.name, sharing = EXCLUDED.sharing, group_ids = EXCLUDED.group_ids, widgets = EXCLUDED.widgets, created_at = EXCLUDED.created_at, updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsDashboard :one
SELECT EXISTS (SELECT 1 FROM dashboard WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearDashboard :execrows
DELETE FROM dashboard;

-- name: ImportDashboardDigest :execrows
INSERT INTO dashboard_digest (id, tenant_id, dashboard_id, account_id, cadence, weekday, hour, time_zone, next_at, last_sent_at, created_at, updated_at)
SELECT r.id, r.tenant_id, r.dashboard_id, r.account_id, r.cadence, r.weekday, r.hour, r.time_zone, r.next_at, r.last_sent_at, r.created_at, r.updated_at
FROM jsonb_populate_record(
  NULL::dashboard_digest,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  dashboard_id = EXCLUDED.dashboard_id, account_id = EXCLUDED.account_id, cadence = EXCLUDED.cadence, weekday = EXCLUDED.weekday, hour = EXCLUDED.hour, time_zone = EXCLUDED.time_zone, next_at = EXCLUDED.next_at, last_sent_at = EXCLUDED.last_sent_at, created_at = EXCLUDED.created_at, updated_at = EXCLUDED.updated_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsDashboardDigest :one
SELECT EXISTS (SELECT 1 FROM dashboard_digest WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearDashboardDigest :execrows
DELETE FROM dashboard_digest;

-- name: ImportAgentModel :execrows
INSERT INTO agent_model (id, tenant_id, key, name, source, offered_id, model, region, retention_days, consent_by, consent_at, revision, retired, created_at, updated_at, version)
SELECT r.id, r.tenant_id, r.key, r.name, r.source, r.offered_id, r.model, r.region, r.retention_days, r.consent_by, r.consent_at, r.revision, r.retired, r.created_at, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::agent_model,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  key = EXCLUDED.key, name = EXCLUDED.name, source = EXCLUDED.source, offered_id = EXCLUDED.offered_id, model = EXCLUDED.model, region = EXCLUDED.region, retention_days = EXCLUDED.retention_days, consent_by = EXCLUDED.consent_by, consent_at = EXCLUDED.consent_at, revision = EXCLUDED.revision, retired = EXCLUDED.retired, created_at = EXCLUDED.created_at, updated_at = EXCLUDED.updated_at, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsAgentModel :one
SELECT EXISTS (SELECT 1 FROM agent_model WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearAgentModel :execrows
DELETE FROM agent_model;

-- name: ImportAgent :execrows
INSERT INTO agent (id, tenant_id, account_id, name, purpose, kind, owner_id, risk_class, scopes, model_id, policy_reversible, policy_irreversible, tokens_per_day, actions_per_hour, concurrent_runs, min_accuracy, status, approved_by, approved_at, approved_model_revision, suspended_by, suspended_at, created_by, created_at, updated_at, version)
SELECT r.id, r.tenant_id, r.account_id, r.name, r.purpose, r.kind, r.owner_id, r.risk_class, r.scopes, r.model_id, r.policy_reversible, r.policy_irreversible, r.tokens_per_day, r.actions_per_hour, r.concurrent_runs, r.min_accuracy, r.status, r.approved_by, r.approved_at, r.approved_model_revision, r.suspended_by, r.suspended_at, r.created_by, r.created_at, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::agent,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  account_id = EXCLUDED.account_id, name = EXCLUDED.name, purpose = EXCLUDED.purpose, kind = EXCLUDED.kind, owner_id = EXCLUDED.owner_id, risk_class = EXCLUDED.risk_class, scopes = EXCLUDED.scopes, model_id = EXCLUDED.model_id, policy_reversible = EXCLUDED.policy_reversible, policy_irreversible = EXCLUDED.policy_irreversible, tokens_per_day = EXCLUDED.tokens_per_day, actions_per_hour = EXCLUDED.actions_per_hour, concurrent_runs = EXCLUDED.concurrent_runs, min_accuracy = EXCLUDED.min_accuracy, status = EXCLUDED.status, approved_by = EXCLUDED.approved_by, approved_at = EXCLUDED.approved_at, approved_model_revision = EXCLUDED.approved_model_revision, suspended_by = EXCLUDED.suspended_by, suspended_at = EXCLUDED.suspended_at, created_by = EXCLUDED.created_by, created_at = EXCLUDED.created_at, updated_at = EXCLUDED.updated_at, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsAgent :one
SELECT EXISTS (SELECT 1 FROM agent WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearAgent :execrows
DELETE FROM agent;

-- name: ImportAgentHalt :execrows
INSERT INTO agent_halt (id, tenant_id, halted, set_by, set_at)
SELECT r.id, r.tenant_id, r.halted, r.set_by, r.set_at
FROM jsonb_populate_record(
  NULL::agent_halt,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id) DO UPDATE SET
  id = EXCLUDED.id, halted = EXCLUDED.halted, set_by = EXCLUDED.set_by, set_at = EXCLUDED.set_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsAgentHalt :one
SELECT EXISTS (SELECT 1 FROM agent_halt) AS held;

-- name: ClearAgentHalt :execrows
DELETE FROM agent_halt;

-- name: ImportAgentRequest :execrows
INSERT INTO agent_request (id, tenant_id, agent_id, use_case, class, input, status, decided_by, decided_at, code, created_at, version)
SELECT r.id, r.tenant_id, r.agent_id, r.use_case, r.class, r.input, r.status, r.decided_by, r.decided_at, r.code, r.created_at, r.version
FROM jsonb_populate_record(
  NULL::agent_request,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id, use_case = EXCLUDED.use_case, class = EXCLUDED.class, input = EXCLUDED.input, status = EXCLUDED.status, decided_by = EXCLUDED.decided_by, decided_at = EXCLUDED.decided_at, code = EXCLUDED.code, created_at = EXCLUDED.created_at, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsAgentRequest :one
SELECT EXISTS (SELECT 1 FROM agent_request WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearAgentRequest :execrows
DELETE FROM agent_request;

-- name: ImportAgentAction :execrows
INSERT INTO agent_action (id, tenant_id, agent_id, use_case, class, decision, code, tokens, request_id, at)
SELECT r.id, r.tenant_id, r.agent_id, r.use_case, r.class, r.decision, r.code, r.tokens, r.request_id, r.at
FROM jsonb_populate_record(
  NULL::agent_action,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id, use_case = EXCLUDED.use_case, class = EXCLUDED.class, decision = EXCLUDED.decision, code = EXCLUDED.code, tokens = EXCLUDED.tokens, request_id = EXCLUDED.request_id, at = EXCLUDED.at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsAgentAction :one
SELECT EXISTS (SELECT 1 FROM agent_action WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearAgentAction :execrows
DELETE FROM agent_action;

-- name: ImportAgentEvalCase :execrows
INSERT INTO agent_eval_case (id, tenant_id, agent_id, title, notes, expected, created_by, created_at)
SELECT r.id, r.tenant_id, r.agent_id, r.title, r.notes, r.expected, r.created_by, r.created_at
FROM jsonb_populate_record(
  NULL::agent_eval_case,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id, title = EXCLUDED.title, notes = EXCLUDED.notes, expected = EXCLUDED.expected, created_by = EXCLUDED.created_by, created_at = EXCLUDED.created_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsAgentEvalCase :one
SELECT EXISTS (SELECT 1 FROM agent_eval_case WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearAgentEvalCase :execrows
DELETE FROM agent_eval_case;

-- name: ImportAgentEvalRun :execrows
INSERT INTO agent_eval_run (id, tenant_id, agent_id, model_revision, cases, passed, run_by, at)
SELECT r.id, r.tenant_id, r.agent_id, r.model_revision, r.cases, r.passed, r.run_by, r.at
FROM jsonb_populate_record(
  NULL::agent_eval_run,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id, model_revision = EXCLUDED.model_revision, cases = EXCLUDED.cases, passed = EXCLUDED.passed, run_by = EXCLUDED.run_by, at = EXCLUDED.at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsAgentEvalRun :one
SELECT EXISTS (SELECT 1 FROM agent_eval_run WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearAgentEvalRun :execrows
DELETE FROM agent_eval_run;

-- name: ImportAiProvenanceMark :execrows
INSERT INTO ai_provenance (id, tenant_id, target_type, target_id, fields, suggestion_id, agent_id, model, prompt_id, prompt_version, accepted_by, accepted_at)
SELECT r.id, r.tenant_id, r.target_type, r.target_id, r.fields, r.suggestion_id, r.agent_id, r.model, r.prompt_id, r.prompt_version, r.accepted_by, r.accepted_at
FROM jsonb_populate_record(
  NULL::ai_provenance,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  target_type = EXCLUDED.target_type, target_id = EXCLUDED.target_id, fields = EXCLUDED.fields, suggestion_id = EXCLUDED.suggestion_id, agent_id = EXCLUDED.agent_id, model = EXCLUDED.model, prompt_id = EXCLUDED.prompt_id, prompt_version = EXCLUDED.prompt_version, accepted_by = EXCLUDED.accepted_by, accepted_at = EXCLUDED.accepted_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsAiProvenanceMark :one
SELECT EXISTS (SELECT 1 FROM ai_provenance WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearAiProvenanceMark :execrows
DELETE FROM ai_provenance;

-- name: ImportAgentJob :execrows
INSERT INTO agent_job (id, tenant_id, agent_id, target_type, target_id, status, code, proposals, tokens,
  started_by, started_at, ended_at)
SELECT r.id, r.tenant_id, r.agent_id, r.target_type, r.target_id, r.status, r.code, r.proposals, r.tokens,
  r.started_by, r.started_at, r.ended_at
FROM jsonb_populate_record(
  NULL::agent_job,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  agent_id = EXCLUDED.agent_id, target_type = EXCLUDED.target_type, target_id = EXCLUDED.target_id,
  status = EXCLUDED.status, code = EXCLUDED.code, proposals = EXCLUDED.proposals, tokens = EXCLUDED.tokens,
  started_by = EXCLUDED.started_by, started_at = EXCLUDED.started_at, ended_at = EXCLUDED.ended_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsAgentJob :one
SELECT EXISTS (SELECT 1 FROM agent_job WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearAgentJob :execrows
DELETE FROM agent_job;

-- name: ImportPageMedia :execrows
INSERT INTO page_media (tenant_id, page_id, media_id)
SELECT r.tenant_id, r.page_id, r.media_id
FROM jsonb_populate_record(
  NULL::page_media,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, page_id, media_id) DO NOTHING;

-- name: HoldsPageMedia :one
SELECT EXISTS (
  SELECT 1 FROM page_media
  WHERE page_id = (sqlc.arg('payload')::jsonb->>'page_id')::uuid
    AND media_id = (sqlc.arg('payload')::jsonb->>'media_id')::uuid
) AS held;

-- name: ClearPageMedia :execrows
DELETE FROM page_media;

-- name: ImportWhiteboardComment :execrows
INSERT INTO whiteboard_comment (
  id, tenant_id, whiteboard_id, parent_id, author_id, body, element_id, x, y, resolved_at, resolved_by,
  created_at, edited_at, deleted_at, version
)
SELECT
  r.id, r.tenant_id, r.whiteboard_id, r.parent_id, r.author_id, r.body, r.element_id, r.x, r.y, r.resolved_at,
  r.resolved_by, r.created_at, r.edited_at, r.deleted_at, r.version
FROM jsonb_populate_record(
  NULL::whiteboard_comment,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  body = EXCLUDED.body,
  resolved_at = EXCLUDED.resolved_at,
  resolved_by = EXCLUDED.resolved_by,
  edited_at = EXCLUDED.edited_at,
  deleted_at = EXCLUDED.deleted_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsWhiteboardComment :one
SELECT EXISTS (SELECT 1 FROM whiteboard_comment WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearWhiteboardComment :execrows
DELETE FROM whiteboard_comment;

-- name: ImportCmdbAffected :execrows
INSERT INTO cmdb_affected (id, tenant_id, item_id, ci_id, added_by, added_at)
SELECT r.id, r.tenant_id, r.item_id, r.ci_id, r.added_by, r.added_at
FROM jsonb_populate_record(
  NULL::cmdb_affected,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  added_by = EXCLUDED.added_by,
  added_at = EXCLUDED.added_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCmdbAffected :one
SELECT EXISTS (SELECT 1 FROM cmdb_affected WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCmdbAffected :execrows
DELETE FROM cmdb_affected;

-- name: ImportCmdbCampaign :execrows
INSERT INTO cmdb_campaign (id, tenant_id, hub_id, name, classes, due_by, due_on, review_months, started_by, started_at, closed_at, closed_by, version)
SELECT r.id, r.tenant_id, r.hub_id, r.name, r.classes, r.due_by, r.due_on, r.review_months, r.started_by, r.started_at, r.closed_at, r.closed_by, r.version
FROM jsonb_populate_record(
  NULL::cmdb_campaign,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  classes = EXCLUDED.classes,
  due_by = EXCLUDED.due_by,
  due_on = EXCLUDED.due_on,
  review_months = EXCLUDED.review_months,
  closed_at = EXCLUDED.closed_at,
  closed_by = EXCLUDED.closed_by,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCmdbCampaign :one
SELECT EXISTS (SELECT 1 FROM cmdb_campaign WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCmdbCampaign :execrows
DELETE FROM cmdb_campaign;

-- name: ImportCmdbAttestation :execrows
INSERT INTO cmdb_attestation (tenant_id, campaign_id, ci_id, owner_id, verdict, attested_by, attested_at)
SELECT r.tenant_id, r.campaign_id, r.ci_id, r.owner_id, r.verdict, r.attested_by, r.attested_at
FROM jsonb_populate_record(
  NULL::cmdb_attestation,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, campaign_id, ci_id) DO UPDATE SET
  owner_id = EXCLUDED.owner_id,
  verdict = EXCLUDED.verdict,
  attested_by = EXCLUDED.attested_by,
  attested_at = EXCLUDED.attested_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCmdbAttestation :one
SELECT EXISTS (SELECT 1 FROM cmdb_attestation WHERE campaign_id = (sqlc.arg('payload')::jsonb->>'campaign_id')::uuid AND ci_id = (sqlc.arg('payload')::jsonb->>'ci_id')::uuid) AS held;

-- name: ClearCmdbAttestation :execrows
DELETE FROM cmdb_attestation;

-- name: ImportCmdbCiGroup :execrows
INSERT INTO cmdb_ci_group (id, tenant_id, hub_id, key, name, filter, service_id, updated_by, updated_at, version)
SELECT r.id, r.tenant_id, r.hub_id, r.key, r.name, r.filter, r.service_id, r.updated_by, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::cmdb_ci_group,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  filter = EXCLUDED.filter,
  service_id = EXCLUDED.service_id,
  updated_by = EXCLUDED.updated_by,
  updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCmdbCiGroup :one
SELECT EXISTS (SELECT 1 FROM cmdb_ci_group WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCmdbCiGroup :execrows
DELETE FROM cmdb_ci_group;

-- name: ImportCmdbAsset :execrows
INSERT INTO cmdb_asset (ci_id, tenant_id, hub_id, asset_tag, cost_minor, currency, purchased_on, supplier, warranty_until, contract_id, stockroom_id, assigned_to, request_id, state, disposal, updated_by, updated_at, version)
SELECT r.ci_id, r.tenant_id, r.hub_id, r.asset_tag, r.cost_minor, r.currency, r.purchased_on, r.supplier, r.warranty_until, r.contract_id, r.stockroom_id, r.assigned_to, r.request_id, r.state, r.disposal, r.updated_by, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::cmdb_asset,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (ci_id) DO UPDATE SET
  asset_tag = EXCLUDED.asset_tag,
  cost_minor = EXCLUDED.cost_minor,
  currency = EXCLUDED.currency,
  purchased_on = EXCLUDED.purchased_on,
  supplier = EXCLUDED.supplier,
  warranty_until = EXCLUDED.warranty_until,
  contract_id = EXCLUDED.contract_id,
  stockroom_id = EXCLUDED.stockroom_id,
  assigned_to = EXCLUDED.assigned_to,
  request_id = EXCLUDED.request_id,
  state = EXCLUDED.state,
  disposal = EXCLUDED.disposal,
  updated_by = EXCLUDED.updated_by,
  updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCmdbAsset :one
SELECT EXISTS (SELECT 1 FROM cmdb_asset WHERE ci_id = (sqlc.arg('payload')::jsonb->>'ci_id')::uuid) AS held;

-- name: ClearCmdbAsset :execrows
DELETE FROM cmdb_asset;

-- name: ImportCmdbSoftwareAlias :execrows
INSERT INTO cmdb_software_alias (id, tenant_id, match_publisher, match_product, publisher, product, updated_by, updated_at, version)
SELECT r.id, r.tenant_id, r.match_publisher, r.match_product, r.publisher, r.product, r.updated_by, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::cmdb_software_alias,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  match_publisher = EXCLUDED.match_publisher,
  match_product = EXCLUDED.match_product,
  publisher = EXCLUDED.publisher,
  product = EXCLUDED.product,
  updated_by = EXCLUDED.updated_by,
  updated_at = EXCLUDED.updated_at,
  version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCmdbSoftwareAlias :one
SELECT EXISTS (SELECT 1 FROM cmdb_software_alias WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCmdbSoftwareAlias :execrows
DELETE FROM cmdb_software_alias;

-- name: ImportApproval :execrows
INSERT INTO approval (id, tenant_id, item_id, workflow_key, step_key, item_version, levels, decline_to, due_hours, current_level, state, requested_by, requested_at, level_since, decided_at, version)
SELECT r.id, r.tenant_id, r.item_id, r.workflow_key, r.step_key, r.item_version, r.levels, r.decline_to, r.due_hours, r.current_level, r.state, r.requested_by, r.requested_at, r.level_since, r.decided_at, r.version
FROM jsonb_populate_record(
  NULL::approval,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET current_level = EXCLUDED.current_level, state = EXCLUDED.state, level_since = EXCLUDED.level_since, decided_at = EXCLUDED.decided_at, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsApproval :one
SELECT EXISTS (SELECT 1 FROM approval WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearApproval :execrows
DELETE FROM approval;

-- name: ImportApprovalDecision :execrows
INSERT INTO approval_decision (id, tenant_id, approval_id, level, approver_id, on_behalf_of, verdict, reason, item_version, decided_at)
SELECT r.id, r.tenant_id, r.approval_id, r.level, r.approver_id, r.on_behalf_of, r.verdict, r.reason, r.item_version, r.decided_at
FROM jsonb_populate_record(
  NULL::approval_decision,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET reason = EXCLUDED.reason
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsApprovalDecision :one
SELECT EXISTS (SELECT 1 FROM approval_decision WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearApprovalDecision :execrows
DELETE FROM approval_decision;

-- name: ImportApprovalDelegation :execrows
INSERT INTO approval_delegation (tenant_id, account_id, delegate_id, starts_at, ends_at)
SELECT r.tenant_id, r.account_id, r.delegate_id, r.starts_at, r.ends_at
FROM jsonb_populate_record(
  NULL::approval_delegation,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, account_id) DO UPDATE SET delegate_id = EXCLUDED.delegate_id, starts_at = EXCLUDED.starts_at, ends_at = EXCLUDED.ends_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsApprovalDelegation :one
SELECT EXISTS (SELECT 1 FROM approval_delegation WHERE account_id = (sqlc.arg('payload')::jsonb->>'account_id')::uuid) AS held;

-- name: ClearApprovalDelegation :execrows
DELETE FROM approval_delegation;

-- name: ImportRelease :execrows
INSERT INTO release (id, tenant_id, hub_id, name, description, release_on, state, released_at,
  notes_page_id, created_at, created_by, updated_at, version)
SELECT r.id, r.tenant_id, r.hub_id, r.name, r.description, r.release_on, r.state, r.released_at,
  r.notes_page_id, r.created_at, r.created_by, r.updated_at, r.version
FROM jsonb_populate_record(
  NULL::release,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name, description = EXCLUDED.description, release_on = EXCLUDED.release_on,
  state = EXCLUDED.state, released_at = EXCLUDED.released_at, notes_page_id = EXCLUDED.notes_page_id,
  updated_at = EXCLUDED.updated_at, version = EXCLUDED.version
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsRelease :one
SELECT EXISTS (SELECT 1 FROM release WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearRelease :execrows
DELETE FROM release;

-- name: ImportReleaseEntry :execrows
INSERT INTO release_entry (tenant_id, release_id, entry_id, added_at)
SELECT r.tenant_id, r.release_id, r.entry_id, r.added_at
FROM jsonb_populate_record(
  NULL::release_entry,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (tenant_id, release_id, entry_id) DO UPDATE SET added_at = EXCLUDED.added_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsReleaseEntry :one
SELECT EXISTS (
  SELECT 1 FROM release_entry
  WHERE release_id = (sqlc.arg('payload')::jsonb->>'release_id')::uuid
    AND entry_id = (sqlc.arg('payload')::jsonb->>'entry_id')::uuid
) AS held;

-- name: ClearReleaseEntry :execrows
DELETE FROM release_entry;

-- name: ImportCodeHook :execrows
INSERT INTO code_hook (id, tenant_id, hub_id, kind, name, enabled, salt, run_as, created_at, updated_at, last_delivery_at)
SELECT r.id, r.tenant_id, r.hub_id, r.kind, r.name, r.enabled, r.salt, r.run_as, r.created_at, r.updated_at,
  r.last_delivery_at
FROM jsonb_populate_record(
  NULL::code_hook,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name, enabled = EXCLUDED.enabled, salt = EXCLUDED.salt, run_as = EXCLUDED.run_as,
  updated_at = EXCLUDED.updated_at, last_delivery_at = EXCLUDED.last_delivery_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCodeHook :one
SELECT EXISTS (SELECT 1 FROM code_hook WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCodeHook :execrows
DELETE FROM code_hook;

-- name: ImportCodeLink :execrows
INSERT INTO code_link (id, tenant_id, item_id, hub_id, hook_id, kind, repository, ref, title, url, author, state,
  happened_at, updated_at)
SELECT r.id, r.tenant_id, r.item_id, r.hub_id, r.hook_id, r.kind, r.repository, r.ref, r.title, r.url, r.author,
  r.state, r.happened_at, r.updated_at
FROM jsonb_populate_record(
  NULL::code_link,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  title = EXCLUDED.title, url = EXCLUDED.url, author = EXCLUDED.author, state = EXCLUDED.state,
  happened_at = EXCLUDED.happened_at, updated_at = EXCLUDED.updated_at
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsCodeLink :one
SELECT EXISTS (SELECT 1 FROM code_link WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearCodeLink :execrows
DELETE FROM code_link;

-- name: ImportFileLink :execrows
INSERT INTO file_link (id, tenant_id, item_id, provider, address, name, added_by, added_at)
SELECT r.id, r.tenant_id, r.item_id, r.provider, r.address, r.name, r.added_by, r.added_at
FROM jsonb_populate_record(
  NULL::file_link,
  sqlc.arg('payload')::jsonb || jsonb_build_object('tenant_id', current_tenant_id())
) r
ON CONFLICT (id) DO UPDATE SET
  provider = EXCLUDED.provider, address = EXCLUDED.address, name = EXCLUDED.name
WHERE sqlc.arg('overwrite')::boolean;

-- name: HoldsFileLink :one
SELECT EXISTS (SELECT 1 FROM file_link WHERE id = (sqlc.arg('payload')::jsonb->>'id')::uuid) AS held;

-- name: ClearFileLink :execrows
DELETE FROM file_link;
