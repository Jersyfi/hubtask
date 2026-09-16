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

-- The devices (N-03, offline-sync.md §6, §10).

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
-- The initial synchronisation (N-02, offline-sync.md §3.1): the current state, one kind at a
-- time, in pages by identifier. Each statement is its kind's Find with the identifier as the page
-- key and the live rows only - the trash and the deleted are tombstones in the log, not state.
-- The column lists mirror the Find statements on purpose, so that the row mappers are shared and
-- a device starting from nothing reads exactly the shape a device that pulled the change would.

-- name: SnapshotContainers :many
SELECT
  c.id, c.tenant_id, c.type, c.parent_id, c.name, c.description, c.icon, c.color_token, c.order_key,
  coalesce(c.policies->>'completion_policy', '')::text AS completion_policy,
  aap.strategy AS auto_assign_strategy,
  aap.candidates AS auto_assign_candidates,
  aap.enabled AS auto_assign_enabled,
  c.archived_at, parent.archived_at AS parent_archived_at,
  c.deleted_at, c.trash_batch_id, c.created_by, c.created_at, c.updated_at, c.version
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
  wi.notes, wi.is_completed, wi.completed_at, wi.completed_by, wi.bucket_id, wi.order_key,
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
  wi.content_language, wi.recurrence_rule_id, wi.recurrence_source_id, wi.origin_jumble_id,
  wi.retention_pending_until, wi.retention_rule_id, wi.retention_action,
  wi.retention_blocked_by,
  wi.archived_at, wi.deleted_at, wi.trash_batch_id, wi.created_by, wi.created_at, wi.updated_at,
  wi.version
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

-- The operation log (N-04, offline-sync.md §3.2): what a push did with each op_id, kept for the
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
-- The SYNC_LOG data kind's sweep of the operation log (N-09, data-retention.md §3): an operation
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

-- The clock per field (N-05, offline-sync.md §4.2): the reading of the write that landed, which a
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
-- The workspace's synchronisation epoch (N-11, backup-restore.md §12 B-5): what every cursor is
-- minted under, and what a cursor is judged against.
SELECT sync_epoch FROM tenant WHERE id = current_tenant_id();

-- name: AdvanceSyncEpoch :one
-- A restore into the workspace succeeded: every cursor minted before is from an older epoch now,
-- and the device holding one resynchronises from scratch.
UPDATE tenant SET sync_epoch = sync_epoch + 1 WHERE id = current_tenant_id()
RETURNING sync_epoch;
