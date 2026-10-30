-- The change log: state deltas for offline clients (offline-sync.md §10). Deliberately separate
-- from the outbox - different recipients, different retention, different compatibility
-- commitments.

-- name: RecordChange :exec
-- `seq` is assigned by the database (GENERATED ALWAYS AS IDENTITY), because the cursor a client
-- pages on has to be gapless and monotonic per tenant. A value chosen in the application would
-- leave holes wherever a transaction rolled back, and a client would then wait for a change that
-- is never coming.
INSERT INTO change_log (
  tenant_id, entity, entity_id, op, container_id, actor_id, device_id, hlc, occurred_at, payload
) VALUES (
  current_tenant_id(), sqlc.arg('entity'), sqlc.arg('entity_id'), sqlc.arg('op'),
  sqlc.narg('container_id'), sqlc.narg('actor_id'), sqlc.narg('device_id'),
  sqlc.arg('hlc'), sqlc.arg('occurred_at'), sqlc.narg('payload')
);

-- name: ReadChangesAfter :many
-- One batch of the change log, in cursor order. What the stream sends and what `:pull` will page.
--
-- `seq > $1` and nothing else: the cursor is a position in a monotonic sequence, so a walk needs no
-- offset, no timestamp comparison and no second sort key. The identity is table-wide rather than
-- per tenant, which makes the sequence sparse for any one of them - a gap between two of a tenant's
-- rows is somebody else's row, never a row of theirs that is missing.
SELECT seq, entity, entity_id, op, container_id, actor_id, device_id, hlc, occurred_at, payload
FROM change_log
WHERE tenant_id = current_tenant_id() AND seq > sqlc.arg('after')
ORDER BY seq
LIMIT sqlc.arg('batch');

-- name: LatestChangeSeq :one
-- Where the log stands now, which is where a client with no cursor starts.
--
-- Zero for a tenant that has never changed anything, and that is the right answer rather than a
-- missing one: a stream opened on an untouched workspace resumes from the beginning of a sequence
-- that has not started, and the first change it is told about is the first change there is.
SELECT coalesce(max(seq), 0)::bigint FROM change_log WHERE tenant_id = current_tenant_id();

-- The devices (offline-sync.md §6, §10).

-- name: TouchDevice :one
-- Registration and every contact after it in one statement. The insert is the registration; the
-- update is the contact, and it is refused - no row comes back - when the identifier belongs to
-- another account or the device was forgotten, which the adapter then tells apart. What the
-- device said about itself replaces what stood; what it left out leaves it alone.
INSERT INTO sync_device (
  id, tenant_id, account_id, platform, display_name, last_cursor, last_seen_at, credential_id,
  created_at
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'),
  sqlc.narg('platform'), sqlc.narg('display_name'), sqlc.narg('last_cursor'),
  sqlc.arg('now'), sqlc.narg('credential_id'), sqlc.arg('now')
)
ON CONFLICT (id) DO UPDATE SET
  platform      = coalesce(excluded.platform, sync_device.platform),
  display_name  = coalesce(excluded.display_name, sync_device.display_name),
  last_cursor   = greatest(excluded.last_cursor, sync_device.last_cursor),
  last_seen_at  = excluded.last_seen_at,
  credential_id = coalesce(excluded.credential_id, sync_device.credential_id)
WHERE sync_device.tenant_id = current_tenant_id()
  AND sync_device.account_id = excluded.account_id
  AND NOT sync_device.blocked
RETURNING id, tenant_id, account_id, platform, display_name, last_cursor, last_seen_at, blocked,
          created_at, credential_id;

-- name: FindDevice :one
SELECT id, tenant_id, account_id, platform, display_name, last_cursor, last_seen_at, blocked,
       created_at, credential_id
FROM sync_device
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: ListDevicesOfAccount :many
SELECT id, tenant_id, account_id, platform, display_name, last_cursor, last_seen_at, blocked,
       created_at, credential_id
FROM sync_device
WHERE tenant_id = current_tenant_id() AND account_id = sqlc.arg('account_id')
ORDER BY last_seen_at DESC NULLS LAST, created_at DESC;

-- name: ForgetDevice :one
-- The mark, and the row as it was: what the caller revokes is the credential the row held.
UPDATE sync_device
SET blocked = true, last_seen_at = sqlc.arg('now')
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')
  AND account_id = sqlc.arg('account_id') AND NOT blocked
RETURNING id, tenant_id, account_id, platform, display_name, last_cursor, last_seen_at, blocked,
          created_at, credential_id;

-- name: RevokeSessionsOfStaleDevices :execrows
-- §6's other half: a device silent past the period loses its sign-in. The session the device
-- last synchronised under is revoked before the row goes, in the same pass; a credential that
-- is not a session matches nothing here and revokes nothing.
UPDATE session
SET revoked_at = sqlc.arg('now')
WHERE tenant_id = current_tenant_id() AND revoked_at IS NULL
  AND id IN (
    SELECT credential_id FROM sync_device AS stale
    WHERE stale.tenant_id = current_tenant_id()
      AND coalesce(stale.last_seen_at, stale.created_at) < sqlc.arg('cutoff')
      AND stale.credential_id IS NOT NULL
    ORDER BY coalesce(stale.last_seen_at, stale.created_at)
    LIMIT sqlc.arg('batch')
  );

-- name: DeleteStaleDevices :execrows
-- The DEVICE data kind's sweep (data-retention.md §3: anchor `last_seen_at`). Batched through a
-- subquery, oldest first, DeleteExpiredSessions' shape.
DELETE FROM sync_device
WHERE id IN (
  SELECT id FROM sync_device AS stale
  WHERE stale.tenant_id = current_tenant_id()
    AND coalesce(stale.last_seen_at, stale.created_at) < sqlc.arg('cutoff')
  ORDER BY coalesce(stale.last_seen_at, stale.created_at)
  LIMIT sqlc.arg('batch')
);

-- name: CountStaleDevices :one
SELECT count(*) FROM (
  SELECT 1 FROM sync_device AS stale
  WHERE stale.tenant_id = current_tenant_id()
    AND coalesce(stale.last_seen_at, stale.created_at) < sqlc.arg('cutoff')
  LIMIT sqlc.arg('ceiling')
) AS due;
-- The initial synchronisation (offline-sync.md §3.1): the current state, one kind at a
-- time, in pages by identifier. Each statement is its kind's Find with the identifier as the page
-- key and the live rows only - the trash and the deleted are tombstones in the log, not state.
-- The column lists mirror the Find statements on purpose, so that the row mappers are shared and
-- a device starting from nothing reads exactly the shape a device that pulled the change would.

-- name: SnapshotContainers :many
SELECT
  c.id, c.tenant_id, c.type, c.parent_id, c.name, c.description, c.icon, c.color_token, c.order_key,
  coalesce(c.policies->>'completion_policy', '')::text AS completion_policy,
  coalesce(c.policies->'structure', '{}'::jsonb)::text AS structure,
  aap.strategy AS auto_assign_strategy,
  aap.candidates AS auto_assign_candidates,
  aap.enabled AS auto_assign_enabled,
  c.archived_at, parent.archived_at AS parent_archived_at,
  c.deleted_at, c.trash_batch_id, c.created_by, c.created_at, c.updated_at, c.version,
  c.private, c.handle
FROM container c
LEFT JOIN container parent ON parent.id = c.parent_id
LEFT JOIN auto_assign_policy aap ON aap.scope_type = 'COLLECTION' AND aap.scope_id = c.id
WHERE c.tenant_id = current_tenant_id() AND c.deleted_at IS NULL AND c.id > sqlc.arg('after')
ORDER BY c.id
LIMIT sqlc.arg('batch');

-- name: SnapshotBuckets :many
SELECT
  id, tenant_id, collection_id, name, order_key, wip_limit, is_done_bucket, color_token,
  deleted_at, version
FROM bucket
WHERE tenant_id = current_tenant_id() AND deleted_at IS NULL AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotLabels :many
SELECT
  id, tenant_id, collection_id, name, color_token, description, deleted_at, version
FROM label
WHERE tenant_id = current_tenant_id() AND deleted_at IS NULL AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotWorkItems :many
SELECT
  wi.id, wi.tenant_id, wi.collection_id, wi.type, wi.parent_id, wi.path, wi.depth, wi.title,
  wi.amount, wi.notes, wi.is_completed, wi.completed_at, wi.completed_by, wi.completion_count,
  wi.bucket_id, wi.order_key,
  wi.assignee_id, wi.start_at, wi.due_at, wi.due_date_only, wi.due_time_zone,
  wi.cover_kind, wi.cover_color_token, wi.cover_media_id,
  (SELECT coalesce(jsonb_object_agg(kv.key, kv.value), '{}'::jsonb)
     FROM jsonb_each(wi.custom_fields) AS kv
    WHERE EXISTS (
      SELECT 1 FROM custom_field_definition cfd
       WHERE cfd.deleted_at IS NULL
         AND cfd.id = (wi.custom_field_refs ->> kv.key)::uuid
         AND (cfd.collection_id = wi.collection_id OR cfd.collection_id IS NULL)
    ))::jsonb AS custom_fields,
  wi.content_language, wi.calendar_uid, wi.recurrence_rule_id, wi.recurrence_source_id,
  wi.origin_jumble_id,
  wi.retention_pending_until, wi.retention_rule_id, wi.retention_action,
  wi.retention_blocked_by,
  wi.archived_at, wi.deleted_at, wi.trash_batch_id, wi.created_by, wi.created_at, wi.updated_at,
  wi.version, wi.number, wi.entry_type, wi.status
FROM work_item wi
WHERE wi.tenant_id = current_tenant_id() AND wi.deleted_at IS NULL AND wi.id > sqlc.arg('after')
ORDER BY wi.id
LIMIT sqlc.arg('batch');

-- name: SnapshotSetElements :many
-- Every tag row of every live entry, keyed the way the table is. Both tags travel: a removed
-- element with its removal tag is what lets a device merge a later re-add correctly
-- (core/domain/model/work/SetElement.go).
SELECT se.item_id, se.set_name, se.element_id, se.add_tag, se.remove_tag, wi.collection_id
FROM set_element se
JOIN work_item wi ON wi.tenant_id = se.tenant_id AND wi.id = se.item_id
WHERE se.tenant_id = current_tenant_id() AND wi.deleted_at IS NULL
  AND (se.item_id, se.set_name, se.element_id) >
      (sqlc.arg('after_item_id')::uuid, sqlc.arg('after_set_name')::text, sqlc.arg('after_element_id')::uuid)
ORDER BY se.item_id, se.set_name, se.element_id
LIMIT sqlc.arg('batch');

-- name: SnapshotComments :many
SELECT c.id, c.tenant_id, c.item_id, c.author_id, c.parent_comment_id, c.body,
       c.created_at, c.edited_at, c.deleted_at, c.version, c.kind, c.system_code, c.system_params,
       wi.collection_id
FROM comment c
JOIN work_item wi ON wi.tenant_id = c.tenant_id AND wi.id = c.item_id
WHERE c.tenant_id = current_tenant_id() AND c.deleted_at IS NULL AND wi.deleted_at IS NULL
  AND c.id > sqlc.arg('after')
ORDER BY c.id
LIMIT sqlc.arg('batch');

-- name: SnapshotReminders :many
SELECT r.id, r.tenant_id, r.item_id, r.offset_spec, r.channels, r.recipients, r.state, r.fire_at,
       r.created_at, r.updated_at, r.version, wi.collection_id
FROM reminder r
JOIN work_item wi ON wi.tenant_id = r.tenant_id AND wi.id = r.item_id
WHERE r.tenant_id = current_tenant_id() AND wi.deleted_at IS NULL AND r.id > sqlc.arg('after')
ORDER BY r.id
LIMIT sqlc.arg('batch');

-- name: SnapshotRecurrenceRules :many
SELECT rr.id, rr.tenant_id, rr.source_item_id, rr.rrule, rr.time_zone, rr.mode, rr.horizon_days,
       rr.ends_at, rr.max_count, rr.last_materialized_at, rr.created_at, rr.updated_at, rr.version,
       wi.collection_id
FROM recurrence_rule rr
JOIN work_item wi ON wi.tenant_id = rr.tenant_id AND wi.id = rr.source_item_id
WHERE rr.tenant_id = current_tenant_id() AND wi.deleted_at IS NULL AND rr.id > sqlc.arg('after')
ORDER BY rr.id
LIMIT sqlc.arg('batch');

-- name: SnapshotTemplates :many
SELECT id, tenant_id, scope_type, scope_id, name, description, root_type, nodes,
       created_at, updated_at, deleted_at, version
FROM template
WHERE tenant_id = current_tenant_id() AND deleted_at IS NULL AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotEntryTypes :many
-- The workspace's type rows (migration 0209); the system types it never changed are the domain's.
SELECT id, tenant_id, key, level, system, name, icon, color_token, completable, workflow, record,
  schedulable, estimable, allowed_children, position, archived_at, created_at, updated_at, version, form
FROM entry_type
WHERE tenant_id = current_tenant_id() AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- The operation log (offline-sync.md §3.2): what a push did with each op_id, kept for the
-- offline window so that a repeated push takes effect exactly once.

-- name: FindSyncOp :one
SELECT op_id, device_id, result, entity_id, applied_at, response
FROM sync_op_log
WHERE tenant_id = current_tenant_id() AND op_id = sqlc.arg('op_id');

-- name: RecordSyncOp :exec
-- Written in the transaction that applied the mutation. A conflict is a repeat that raced this
-- one and is left standing: the first answer is the answer.
INSERT INTO sync_op_log (tenant_id, op_id, device_id, result, entity_id, applied_at, response)
VALUES (current_tenant_id(), sqlc.arg('op_id'), sqlc.narg('device_id'), sqlc.arg('result'),
        sqlc.narg('entity_id'), sqlc.arg('applied_at'), sqlc.narg('response'))
ON CONFLICT (tenant_id, op_id) DO NOTHING;

-- name: DeleteAgedSyncOps :execrows
-- The SYNC_LOG data kind's sweep of the operation log (data-retention.md §3): an operation
-- past the offline window has answered every repeat it will ever see - a device silent longer
-- than the window resynchronises from scratch. Batched through a subquery, oldest first,
-- DeleteStaleDevices' shape; the primary key is what the subquery hands back.
DELETE FROM sync_op_log
WHERE (tenant_id, op_id) IN (
  SELECT aged.tenant_id, aged.op_id FROM sync_op_log AS aged
  WHERE aged.tenant_id = current_tenant_id() AND aged.applied_at < sqlc.arg('cutoff')
  ORDER BY aged.applied_at
  LIMIT sqlc.arg('batch')
);

-- name: CountAgedSyncOps :one
SELECT count(*) FROM (
  SELECT 1 FROM sync_op_log AS aged
  WHERE aged.tenant_id = current_tenant_id() AND aged.applied_at < sqlc.arg('cutoff')
  LIMIT sqlc.arg('ceiling')
) AS due;

-- name: DeleteAgedTombstones :execrows
-- A tombstone past the window has told every device that could still be told (offline-sync.md
-- §7); its own purge date is the deletion plus the window as it stood, and the cutoff is the
-- window as it stands, so the later of the two is what is honoured.
DELETE FROM tombstone
WHERE (tenant_id, entity, entity_id) IN (
  SELECT aged.tenant_id, aged.entity, aged.entity_id FROM tombstone AS aged
  WHERE aged.tenant_id = current_tenant_id()
    AND aged.deleted_at < sqlc.arg('cutoff') AND aged.purge_after < sqlc.arg('now')
  ORDER BY aged.deleted_at
  LIMIT sqlc.arg('batch')
);

-- name: CountAgedTombstones :one
SELECT count(*) FROM (
  SELECT 1 FROM tombstone AS aged
  WHERE aged.tenant_id = current_tenant_id()
    AND aged.deleted_at < sqlc.arg('cutoff') AND aged.purge_after < sqlc.arg('now')
  LIMIT sqlc.arg('ceiling')
) AS due;

-- name: HoldsTombstone :one
-- Whether an entity has been purged (offline-sync.md §7). The trash is not a tombstone: a trashed
-- entry can still be restored, and the use case that receives a mutation about it says so itself.
SELECT EXISTS (
  SELECT 1 FROM tombstone
  WHERE tenant_id = current_tenant_id() AND entity = sqlc.arg('entity') AND entity_id = sqlc.arg('entity_id')
)::boolean AS held;

-- The clock per field (offline-sync.md §4.2): the reading of the write that landed, which a
-- push's reading is compared against per field.

-- name: StampFieldClock :exec
-- The reading of the write that landed replaces what stood: the writer decided, and the row
-- records the decision. A guard that kept an older row would let a device outvote an edit made
-- after it by a clock that was merely ahead.
INSERT INTO field_clock (tenant_id, entity, entity_id, field, hlc)
VALUES (current_tenant_id(), sqlc.arg('entity'), sqlc.arg('entity_id'), sqlc.arg('field'), sqlc.arg('hlc'))
ON CONFLICT (tenant_id, entity, entity_id, field) DO UPDATE SET hlc = excluded.hlc;

-- name: FieldClocksOf :many
-- Every field of one entity that has a reading, for the merge to compare against.
SELECT field, hlc
FROM field_clock
WHERE tenant_id = current_tenant_id() AND entity = sqlc.arg('entity') AND entity_id = sqlc.arg('entity_id');

-- name: CurrentSyncEpoch :one
-- The workspace's synchronisation epoch (backup-restore.md §12 B-5): what every cursor is
-- minted under, and what a cursor is judged against.
SELECT sync_epoch FROM tenant WHERE id = current_tenant_id();

-- name: AdvanceSyncEpoch :one
-- A restore into the workspace succeeded: every cursor minted before is from an older epoch now,
-- and the device holding one resynchronises from scratch.
UPDATE tenant SET sync_epoch = sync_epoch + 1 WHERE id = current_tenant_id()
RETURNING sync_epoch;

-- name: SnapshotWorkflows :many
-- The workspace's workflow rows (migration 0215); the shipped one it never used is the domain's.
SELECT id, tenant_id, key, system, name, published, draft, published_at, published_by,
  created_at, updated_at, version
FROM workflow
WHERE tenant_id = current_tenant_id() AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotLinkTypes :many
-- The workspace's link type rows (migration 0213); the six shipped ones are the domain's.
SELECT id, tenant_id, key, name, inverse_name, direction, cardinality, cycles, waits, position,
  archived_at, created_at, updated_at, version
FROM link_type
WHERE tenant_id = current_tenant_id() AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotEntryLinks :many
-- Every link between two living entries, with the collection of each end: a reader is told of a
-- link only when they may read both.
SELECT l.id, l.tenant_id, l.link_type, l.source_id, l.target_id, l.created_by, l.created_at,
  l.dependency_kind, l.lag_days,
  s.collection_id AS source_collection_id, t.collection_id AS target_collection_id
FROM entry_link l
JOIN work_item s ON s.tenant_id = l.tenant_id AND s.id = l.source_id AND s.deleted_at IS NULL
JOIN work_item t ON t.tenant_id = l.tenant_id AND t.id = l.target_id AND t.deleted_at IS NULL
WHERE l.tenant_id = current_tenant_id() AND l.id > sqlc.arg('after')
ORDER BY l.id
LIMIT sqlc.arg('batch');

-- name: SnapshotEntrySchedules :many
-- Every live entry's stored scheduling mode and milestone mark (migration 0244), with its
-- collection: a reader is told of it where they may read the entry.
SELECT es.entry_id, es.mode, es.milestone, es.version, wi.collection_id
FROM entry_schedule es
JOIN work_item wi ON wi.tenant_id = es.tenant_id AND wi.id = es.entry_id AND wi.deleted_at IS NULL
WHERE es.tenant_id = current_tenant_id() AND es.entry_id > sqlc.arg('after')
ORDER BY es.entry_id
LIMIT sqlc.arg('batch');

-- name: SnapshotCustomRoles :many
-- The workspace's own roles (migration 0219).
SELECT id, tenant_id, key, name, permissions, created_at, updated_at, version
FROM custom_role
WHERE tenant_id = current_tenant_id() AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotOwnRoleGrants :many
-- The grants of custom roles the reader holds, named to them or to a group they are in: a
-- person's own, never another's (offline-sync.md §4.3).
SELECT g.id, g.tenant_id, g.account_id, g.group_id, g.scope_type, g.scope_id, g.role_id, r.key AS role_key
FROM custom_role_grant g
JOIN custom_role r ON r.tenant_id = g.tenant_id AND r.id = g.role_id
WHERE g.tenant_id = current_tenant_id() AND g.id > sqlc.arg('after')
  AND (g.account_id = sqlc.arg('account_id')::uuid
    OR g.group_id IN (
      SELECT m.group_id FROM account_group_member m
      WHERE m.tenant_id = current_tenant_id() AND m.account_id = sqlc.arg('account_id')::uuid))
ORDER BY g.id
LIMIT sqlc.arg('batch');

-- name: IsAccountGroupMember :one
-- Whether the account is in the group: a group's grant reaches its members' devices only.
SELECT EXISTS (
  SELECT 1 FROM account_group_member
  WHERE tenant_id = current_tenant_id() AND group_id = sqlc.arg('group_id') AND account_id = sqlc.arg('account_id')
) AS member;

-- name: SnapshotPages :many
-- The workspace's living pages with their bodies (migration 0231), for reading offline; each is
-- filed under its hub, which decides who is told of it. A page behind a restriction stays on the
-- server: a hub's readers are not all its readers (migration 0261).
SELECT p.id, p.hub_id, p.parent_id, p.title, p.body, p.icon, p.order_key, p.template, p.content_language,
       p.owner_id, p.verified_until, p.verified_at, p.verified_by, p.archived_at, p.deleted_at,
       p.created_by, p.created_at, p.updated_by, p.updated_at, p.version, p.restricted, p.daily_for
FROM page p
WHERE p.tenant_id = current_tenant_id() AND p.id > sqlc.arg('after') AND p.deleted_at IS NULL
  AND NOT page_behind_restriction(p.id)
ORDER BY p.id
LIMIT sqlc.arg('batch');

-- name: SnapshotCmdbCis :many
-- The workspace's configuration items not merged away (migration 0236), for reading a hub's
-- inventory offline; each is filed under its hub, which decides who is told of it.
SELECT id, hub_id, class_key, name, stage, status, attributes, provenance, owner_account_id, owner_group_id,
       location_ci_id, protection, review_due, quantity, last_seen_at, merged_into,
       created_by, created_at, updated_by, updated_at, version
FROM cmdb_ci
WHERE tenant_id = current_tenant_id() AND id > sqlc.arg('after') AND merged_into IS NULL
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotCmdbRelations :many
-- The relations between them, each filed under its hub.
SELECT id, hub_id, type_key, source_ci_id, target_ci_id, data_source, first_seen, last_seen, created_by
FROM cmdb_relation
WHERE tenant_id = current_tenant_id() AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotGoalPeriods :many
-- The workspace's goal periods (migration 0248), workspace-wide: read where the workspace is.
SELECT id, name, start_on, end_on, state, version
FROM goal_period
WHERE tenant_id = current_tenant_id() AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotGoals :many
-- The workspace's goals as their definitions, each filed under its hub - none for the workspace's.
SELECT id, period_id, hub_id, parent_id, title, description, owner_id, status, confidence,
       key_results, grade, version
FROM goal
WHERE tenant_id = current_tenant_id() AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotGoalCheckIns :many
-- The goals' check-ins, each with its goal's hub, which decides who is told of it.
SELECT c.id, c.goal_id, c.author_id, c.status, c.confidence, c.note, c."values", c.created_at, g.hub_id
FROM goal_check_in c
JOIN goal g ON g.tenant_id = c.tenant_id AND g.id = c.goal_id
WHERE c.tenant_id = current_tenant_id() AND c.id > sqlc.arg('after')
ORDER BY c.id
LIMIT sqlc.arg('batch');

-- name: SnapshotDashboards :many
-- The workspace's dashboards as their widgets' queries, each filed under its hub; a private one
-- reaches its owner alone.
SELECT id, hub_id, owner_id, name, sharing, group_ids, widgets, version
FROM dashboard
WHERE tenant_id = current_tenant_id() AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotSprints :many
-- The workspace's sprints (migration 0245), each filed under its collection.
SELECT id, collection_id, name, goal, start_on, end_on, state, unit, committed_value,
  committed_count, completed_value, completed_count, started_at, closed_at, created_at,
  created_by, updated_at, version
FROM sprint
WHERE tenant_id = current_tenant_id() AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotSprintEntries :many
-- Every live entry's open sprint, by the entry, with the sprint's collection; a past membership
-- is the sprint's history and stays online.
SELECT se.entry_id, se.sprint_id, se.committed, s.collection_id
FROM sprint_entry se
JOIN sprint s ON s.tenant_id = se.tenant_id AND s.id = se.sprint_id
JOIN work_item wi ON wi.tenant_id = se.tenant_id AND wi.id = se.entry_id AND wi.deleted_at IS NULL
WHERE se.tenant_id = current_tenant_id() AND se.removed_at IS NULL AND se.outcome IS NULL
  AND se.entry_id > sqlc.arg('after')
ORDER BY se.entry_id
LIMIT sqlc.arg('batch');

-- name: SnapshotEntryEstimates :many
-- Every live entry's estimate, with its collection.
SELECT ee.entry_id, ee.unit, ee.value, ee.size, ee.version, wi.collection_id
FROM entry_estimate ee
JOIN work_item wi ON wi.tenant_id = ee.tenant_id AND wi.id = ee.entry_id AND wi.deleted_at IS NULL
WHERE ee.tenant_id = current_tenant_id() AND ee.entry_id > sqlc.arg('after')
ORDER BY ee.entry_id
LIMIT sqlc.arg('batch');

-- name: SnapshotCustomFields :many
-- The workspace's custom field definitions in force, each filed under its collection - none for a
-- workspace-wide one; a deleted one is not walked.
SELECT id, tenant_id, collection_id, key, kind, options, currency, formula, is_required, applies_to,
       created_at, updated_at, deleted_at, version, restricted, read_restricted
FROM custom_field_definition
WHERE tenant_id = current_tenant_id() AND deleted_at IS NULL AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotOwnTimeRecords :many
-- One person's time records (migration 0246), walked for that person alone.
SELECT id, account_id, entry_id, work_type, day, started_at, minutes, note, billable, source,
  corrects, created_at, updated_at, version
FROM time_record
WHERE tenant_id = current_tenant_id() AND account_id = sqlc.arg('account_id')::uuid
  AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotOwnTimesheets :many
-- One person's weeks (migration 0246), walked for that person alone.
SELECT id, account_id, week_start, state, minutes, submitted_at, decided_at, decided_by, reason, version
FROM timesheet
WHERE tenant_id = current_tenant_id() AND account_id = sqlc.arg('account_id')::uuid
  AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotTimeAbsences :many
-- The workspace's absences (migration 0247), the shared calendar every member reads.
SELECT id, account_id, from_day, to_day, kind, half, version
FROM time_absence
WHERE tenant_id = current_tenant_id() AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotTimeBudgets :many
-- The hubs' and collections' budgets (migration 0247), paged by their plan.
SELECT plan_id, minutes, amount_minor, currency, version
FROM time_budget
WHERE tenant_id = current_tenant_id() AND plan_id > sqlc.arg('after')
ORDER BY plan_id
LIMIT sqlc.arg('batch');

-- name: SnapshotPortfolios :many
-- The workspace's portfolios (migration 0248), paged by identifier; their projects are read
-- beside them.
SELECT id, parent_id, name, description, owner_id, status, version
FROM portfolio
WHERE tenant_id = current_tenant_id() AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotAgentModels :many
-- The registry's models (migration 0251), paged by identifier.
SELECT id, key, name, source, offered_id, model, region, retention_days, consent_by, consent_at,
       revision, retired, updated_at, version
FROM agent_model
WHERE tenant_id = current_tenant_id() AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotAgents :many
-- The registry's agents (migration 0251), paged by identifier.
SELECT id, account_id, name, purpose, kind, owner_id, risk_class, scopes, model_id, policy_reversible,
       policy_irreversible, tokens_per_day, actions_per_hour, concurrent_runs, min_accuracy, status,
       approved_by, approved_at, approved_model_revision, suspended_by, suspended_at, created_by,
       created_at, updated_at, version
FROM agent
WHERE tenant_id = current_tenant_id() AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotAgentRequests :many
-- The held acts (migration 0251) with their agent's owner, paged by identifier.
SELECT r.id, r.agent_id, r.use_case, r.class, r.input, r.status, r.decided_by, r.decided_at, r.code,
       r.created_at, r.version, a.owner_id
FROM agent_request r
JOIN agent a ON a.tenant_id = r.tenant_id AND a.id = r.agent_id
WHERE r.tenant_id = current_tenant_id() AND r.id > sqlc.arg('after')
ORDER BY r.id
LIMIT sqlc.arg('batch');

-- name: SnapshotAgentCases :many
-- The evaluation cases (migration 0251), paged by identifier.
SELECT id, agent_id, title, notes, expected, created_by, created_at
FROM agent_eval_case
WHERE tenant_id = current_tenant_id() AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotAgentRuns :many
-- The evaluation runs (migration 0251) with their agent's owner, paged by identifier.
SELECT r.id, r.agent_id, r.model_revision, r.cases, r.passed, r.run_by, r.at, a.owner_id
FROM agent_eval_run r
JOIN agent a ON a.tenant_id = r.tenant_id AND a.id = r.agent_id
WHERE r.tenant_id = current_tenant_id() AND r.id > sqlc.arg('after')
ORDER BY r.id
LIMIT sqlc.arg('batch');

-- name: SnapshotWhiteboards :many
-- The workspace's boards outside the trash with their scenes (migration 0233), for reading offline;
-- each is filed under its hub, which decides who is told of it.
SELECT id, hub_id, title, scene, deleted_at, created_by, created_at, updated_by, updated_at, version
FROM whiteboard
WHERE tenant_id = current_tenant_id() AND id > sqlc.arg('after') AND deleted_at IS NULL
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotCmdbAffected :many
-- The CIs entries name as affected (migration 0267), each filed under the CI's hub.
SELECT a.id, a.item_id, a.ci_id, a.added_by, a.added_at, c.hub_id
FROM cmdb_affected a
JOIN cmdb_ci c ON c.tenant_id = a.tenant_id AND c.id = a.ci_id
WHERE a.tenant_id = current_tenant_id() AND a.id > sqlc.arg('after')
ORDER BY a.id
LIMIT sqlc.arg('batch');

-- name: SnapshotCmdbGroups :many
-- A workspace's CI groups, each filed under its hub.
SELECT id, hub_id, key, name, filter, service_id, updated_by, updated_at, version
FROM cmdb_ci_group
WHERE tenant_id = current_tenant_id() AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotCmdbAssets :many
-- The CIs' asset records, each filed under its hub.
SELECT ci_id, hub_id, asset_tag, cost_minor, currency, purchased_on, supplier, warranty_until, contract_id,
  stockroom_id, assigned_to, request_id, state, disposal, updated_by, updated_at, version
FROM cmdb_asset
WHERE tenant_id = current_tenant_id() AND ci_id > sqlc.arg('after')
ORDER BY ci_id
LIMIT sqlc.arg('batch');

-- name: SnapshotApprovals :many
-- Every live entry's approvals (migration 0268), with the entry's collection, which decides who is
-- told of them.
SELECT a.id, a.tenant_id, a.item_id, a.workflow_key, a.step_key, a.item_version, a.levels, a.decline_to,
       a.due_hours, a.current_level, a.state, a.requested_by, a.requested_at, a.level_since, a.decided_at,
       a.version, wi.collection_id
FROM approval a
JOIN work_item wi ON wi.tenant_id = a.tenant_id AND wi.id = a.item_id AND wi.deleted_at IS NULL
WHERE a.tenant_id = current_tenant_id() AND a.id > sqlc.arg('after')
ORDER BY a.id
LIMIT sqlc.arg('batch');

-- name: SnapshotEntrySlas :many
-- Every live entry's SLA records by goal (migration 0240), with the entry's collection.
SELECT es.id, es.tenant_id, es.item_id, es.goal, es.policy_key, es.target_seconds, es.standing, es.armed_at,
       es.due_at, es.at_risk_at, es.met_at, es.breached_at, es.updated_at, es.version, wi.collection_id
FROM entry_sla es
JOIN work_item wi ON wi.tenant_id = es.tenant_id AND wi.id = es.item_id AND wi.deleted_at IS NULL
WHERE es.tenant_id = current_tenant_id() AND es.id > sqlc.arg('after')
ORDER BY es.id
LIMIT sqlc.arg('batch');

-- name: SnapshotSatisfactionRatings :many
-- Every live request's satisfaction rating (migration 0243), with the request's collection.
SELECT sr.id, sr.tenant_id, sr.item_id, sr.score, sr.rated_at, sr.version, wi.collection_id
FROM satisfaction_rating sr
JOIN work_item wi ON wi.tenant_id = sr.tenant_id AND wi.id = sr.item_id AND wi.deleted_at IS NULL
WHERE sr.tenant_id = current_tenant_id() AND sr.id > sqlc.arg('after')
ORDER BY sr.id
LIMIT sqlc.arg('batch');

-- name: SnapshotReleases :many
-- The workspace's releases (migration 0272), each filed under its hub.
SELECT id, hub_id, name, description, release_on, state, released_at, notes_page_id, created_at,
  created_by, updated_at, version
FROM release
WHERE tenant_id = current_tenant_id() AND id > sqlc.arg('after')
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: SnapshotReleaseEntries :many
-- Every live entry's release, by the entry, with the release's hub.
SELECT re.entry_id, re.release_id, r.hub_id
FROM release_entry re
JOIN release r ON r.tenant_id = re.tenant_id AND r.id = re.release_id
JOIN work_item wi ON wi.tenant_id = re.tenant_id AND wi.id = re.entry_id AND wi.deleted_at IS NULL
WHERE re.tenant_id = current_tenant_id() AND re.entry_id > sqlc.arg('after')
ORDER BY re.entry_id
LIMIT sqlc.arg('batch');

-- name: SnapshotCodeLinks :many
-- Every live entry's commits, branches and pull requests (migration 0275), with the entry's collection.
SELECT cl.id, cl.item_id, cl.hub_id, cl.hook_id, cl.kind, cl.repository, cl.ref, cl.title, cl.url, cl.author,
  cl.state, cl.happened_at, cl.updated_at, wi.collection_id
FROM code_link cl
JOIN work_item wi ON wi.tenant_id = cl.tenant_id AND wi.id = cl.item_id AND wi.deleted_at IS NULL
WHERE cl.tenant_id = current_tenant_id() AND cl.id > sqlc.arg('after')
ORDER BY cl.id
LIMIT sqlc.arg('batch');
