-- The export the archive writer reads a tenant through (E-05, backup-restore.md §3, §5).
--
-- One statement per entity, and they are as alike as the schema lets them be: an identity, when
-- the row last changed, and the row itself as JSON with `tenant_id` taken out - a restore into
-- another tenant must not carry the old one's identifier back in with it. Row level security
-- supplies the tenant condition none of these statements writes (ADR-0010).
--
-- Every statement pages on its own key rather than on OFFSET, so a run that is resumed after
-- process death continues where it stopped instead of counting rows again - and so that a page
-- never repeats or skips a row while the snapshot is open.
--
-- **An entity is exported as a delta only if the schema lets it say when a row changed.** The ones
-- that cannot - the join tables, and the configuration rows that carry a creation stamp and no
-- change stamp - are exported whole in every archive, and a restore replaces their set rather than
-- merging it. Deriving "unchanged" from a column that does not move is how an incremental chain
-- silently loses an edit, and there is no column here to add that would not be a lie until every
-- writer maintained it.

-- name: ExportTenants :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (t.id)::text AS record_id,
         t.updated_at AS changed_at,
         (to_jsonb(t) - 'tenant_id')::jsonb AS payload,
         t.id AS key_1
  FROM tenant t
) s
  WHERE (sqlc.narg('since')::timestamptz IS NULL OR s.changed_at > sqlc.narg('since')::timestamptz)
    AND (s.changed_at, s.key_1) > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_id')::uuid)
  ORDER BY s.changed_at, s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportAccounts :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (a.id)::text AS record_id,
         a.updated_at AS changed_at,
         (to_jsonb(a) - 'tenant_id')::jsonb AS payload,
         a.id AS key_1
  FROM account a
) s
  WHERE (sqlc.narg('since')::timestamptz IS NULL OR s.changed_at > sqlc.narg('since')::timestamptz)
    AND (s.changed_at, s.key_1) > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_id')::uuid)
  ORDER BY s.changed_at, s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportAccountGroups :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (g.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(g) - 'tenant_id')::jsonb AS payload,
         g.id AS key_1
  FROM account_group g
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportAccountGroupMembers :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (m.group_id::text || '/' || m.account_id::text)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(m) - 'tenant_id')::jsonb AS payload,
         m.group_id AS key_1,
         m.account_id AS key_2
  FROM account_group_member m
) s
  WHERE (s.key_1, s.key_2) > (sqlc.arg('after_id')::uuid, sqlc.arg('after_second')::uuid)
  ORDER BY s.key_1, s.key_2
  LIMIT sqlc.arg('batch')::int;

-- name: ExportMemberships :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (m.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(m) - 'tenant_id')::jsonb AS payload,
         m.id AS key_1
  FROM membership m
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportContainers :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (c.id)::text AS record_id,
         c.updated_at AS changed_at,
         (to_jsonb(c) - 'tenant_id')::jsonb AS payload,
         c.id AS key_1
  FROM container c
) s
  WHERE (sqlc.narg('since')::timestamptz IS NULL OR s.changed_at > sqlc.narg('since')::timestamptz)
    AND (s.changed_at, s.key_1) > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_id')::uuid)
  ORDER BY s.changed_at, s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportBuckets :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (b.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(b) - 'tenant_id')::jsonb AS payload,
         b.id AS key_1
  FROM bucket b
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportLabels :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (l.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(l) - 'tenant_id')::jsonb AS payload,
         l.id AS key_1
  FROM label l
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportCustomFieldDefinitions :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (f.id)::text AS record_id,
         f.updated_at AS changed_at,
         (to_jsonb(f) - 'tenant_id')::jsonb AS payload,
         f.id AS key_1
  FROM custom_field_definition f
) s
  WHERE (sqlc.narg('since')::timestamptz IS NULL OR s.changed_at > sqlc.narg('since')::timestamptz)
    AND (s.changed_at, s.key_1) > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_id')::uuid)
  ORDER BY s.changed_at, s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportWorkItems :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (w.id)::text AS record_id,
         w.updated_at AS changed_at,
         (to_jsonb(w) - 'tenant_id')::jsonb AS payload,
         w.id AS key_1
  FROM work_item w
) s
  WHERE (sqlc.narg('since')::timestamptz IS NULL OR s.changed_at > sqlc.narg('since')::timestamptz)
    AND (s.changed_at, s.key_1) > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_id')::uuid)
  ORDER BY s.changed_at, s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportItemLabels :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (il.item_id::text || '/' || il.label_id::text)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(il) - 'tenant_id')::jsonb AS payload,
         il.item_id AS key_1,
         il.label_id AS key_2
  FROM item_label il
) s
  WHERE (s.key_1, s.key_2) > (sqlc.arg('after_id')::uuid, sqlc.arg('after_second')::uuid)
  ORDER BY s.key_1, s.key_2
  LIMIT sqlc.arg('batch')::int;

-- name: ExportItemMembers :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (im.item_id::text || '/' || im.account_id::text)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(im) - 'tenant_id')::jsonb AS payload,
         im.item_id AS key_1,
         im.account_id AS key_2
  FROM item_member im
) s
  WHERE (s.key_1, s.key_2) > (sqlc.arg('after_id')::uuid, sqlc.arg('after_second')::uuid)
  ORDER BY s.key_1, s.key_2
  LIMIT sqlc.arg('batch')::int;

-- name: ExportComments :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (c.id)::text AS record_id,
         GREATEST(c.created_at, COALESCE(c.edited_at, c.created_at), COALESCE(c.deleted_at, c.created_at))::timestamptz AS changed_at,
         (to_jsonb(c) - 'tenant_id')::jsonb AS payload,
         c.id AS key_1
  FROM comment c
) s
  WHERE (sqlc.narg('since')::timestamptz IS NULL OR s.changed_at > sqlc.narg('since')::timestamptz)
    AND (s.changed_at, s.key_1) > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_id')::uuid)
  ORDER BY s.changed_at, s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportActivityEntries :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (a.id)::text AS record_id,
         a.occurred_at AS changed_at,
         (to_jsonb(a) - 'tenant_id')::jsonb AS payload,
         a.id AS key_1
  FROM activity_entry a
) s
  WHERE (sqlc.narg('since')::timestamptz IS NULL OR s.changed_at > sqlc.narg('since')::timestamptz)
    AND (s.changed_at, s.key_1) > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_id')::uuid)
  ORDER BY s.changed_at, s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportMediaObjects :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (m.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(m) - 'tenant_id')::jsonb AS payload,
         m.id AS key_1
  FROM media_object m
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportItemAttachments :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (ia.item_id::text || '/' || ia.media_id::text)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(ia) - 'tenant_id')::jsonb AS payload,
         ia.item_id AS key_1,
         ia.media_id AS key_2
  FROM item_attachment ia
) s
  WHERE (s.key_1, s.key_2) > (sqlc.arg('after_id')::uuid, sqlc.arg('after_second')::uuid)
  ORDER BY s.key_1, s.key_2
  LIMIT sqlc.arg('batch')::int;

-- name: ExportRecurrenceRules :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (r.id)::text AS record_id,
         r.updated_at AS changed_at,
         (to_jsonb(r) - 'tenant_id')::jsonb AS payload,
         r.id AS key_1
  FROM recurrence_rule r
) s
  WHERE (sqlc.narg('since')::timestamptz IS NULL OR s.changed_at > sqlc.narg('since')::timestamptz)
    AND (s.changed_at, s.key_1) > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_id')::uuid)
  ORDER BY s.changed_at, s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportReminders :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (r.id)::text AS record_id,
         r.updated_at AS changed_at,
         (to_jsonb(r) - 'tenant_id')::jsonb AS payload,
         r.id AS key_1
  FROM reminder r
) s
  WHERE (sqlc.narg('since')::timestamptz IS NULL OR s.changed_at > sqlc.narg('since')::timestamptz)
    AND (s.changed_at, s.key_1) > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_id')::uuid)
  ORDER BY s.changed_at, s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportSavedViews :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (v.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(v) - 'tenant_id')::jsonb AS payload,
         v.id AS key_1
  FROM saved_view v
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportTemplates :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (t.id)::text AS record_id,
         t.updated_at AS changed_at,
         (to_jsonb(t) - 'tenant_id')::jsonb AS payload,
         t.id AS key_1
  FROM template t
) s
  WHERE (sqlc.narg('since')::timestamptz IS NULL OR s.changed_at > sqlc.narg('since')::timestamptz)
    AND (s.changed_at, s.key_1) > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_id')::uuid)
  ORDER BY s.changed_at, s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportJumbleEntries :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (j.id)::text AS record_id,
         GREATEST(j.received_at, COALESCE(j.processed_at, j.received_at))::timestamptz AS changed_at,
         (to_jsonb(j) - 'tenant_id')::jsonb AS payload,
         j.id AS key_1
  FROM jumble_entry j
) s
  WHERE (sqlc.narg('since')::timestamptz IS NULL OR s.changed_at > sqlc.narg('since')::timestamptz)
    AND (s.changed_at, s.key_1) > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_id')::uuid)
  ORDER BY s.changed_at, s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportAutoAssignPolicies :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (p.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(p) - 'tenant_id')::jsonb AS payload,
         p.id AS key_1
  FROM auto_assign_policy p
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportAutomationRules :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (r.id)::text AS record_id,
         r.updated_at AS changed_at,
         (to_jsonb(r) - 'tenant_id')::jsonb AS payload,
         r.id AS key_1
  FROM automation_rule r
) s
  WHERE (sqlc.narg('since')::timestamptz IS NULL OR s.changed_at > sqlc.narg('since')::timestamptz)
    AND (s.changed_at, s.key_1) > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_id')::uuid)
  ORDER BY s.changed_at, s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportWebhookSubscriptions :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (s.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(s) - 'tenant_id')::jsonb AS payload,
         s.id AS key_1
  FROM webhook_subscription s
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportCalendarFeeds :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (f.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(f) - 'tenant_id')::jsonb AS payload,
         f.id AS key_1
  FROM calendar_feed f
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportNotificationPreferences :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (p.account_id::text || '/' || p.category || '/' || p.channel)::text AS record_id,
         p.updated_at AS changed_at,
         (to_jsonb(p) - 'tenant_id')::jsonb AS payload,
         p.account_id AS key_1,
         p.category AS key_2,
         p.channel AS key_3
  FROM notification_preference p
) s
  WHERE (sqlc.narg('since')::timestamptz IS NULL OR s.changed_at > sqlc.narg('since')::timestamptz)
    AND (s.changed_at, s.key_1, s.key_2, s.key_3) > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_id')::uuid, sqlc.arg('after_second')::text, sqlc.arg('after_third')::text)
  ORDER BY s.changed_at, s.key_1, s.key_2, s.key_3
  LIMIT sqlc.arg('batch')::int;

-- name: ExportRetentionPolicies :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (p.data_kind)::text AS record_id,
         p.updated_at AS changed_at,
         (to_jsonb(p) - 'tenant_id')::jsonb AS payload,
         p.data_kind AS key_1
  FROM retention_policy p
) s
  WHERE (sqlc.narg('since')::timestamptz IS NULL OR s.changed_at > sqlc.narg('since')::timestamptz)
    AND (s.changed_at, s.key_1) > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_id')::text)
  ORDER BY s.changed_at, s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportConsentRecords :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (c.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(c) - 'tenant_id')::jsonb AS payload,
         c.id AS key_1
  FROM consent_record c
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportLegalHolds :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (h.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(h) - 'tenant_id')::jsonb AS payload,
         h.id AS key_1
  FROM legal_hold h
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportSetElements :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (e.item_id::text || '/' || e.set_name || '/' || e.element_id::text)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(e) - 'tenant_id')::jsonb AS payload,
         e.item_id AS key_1,
         e.set_name AS key_2,
         e.element_id AS key_3
  FROM set_element e
) s
  WHERE (s.key_1, s.key_2, s.key_3) > (sqlc.arg('after_id')::uuid, sqlc.arg('after_second')::text, sqlc.arg('after_third')::uuid)
  ORDER BY s.key_1, s.key_2, s.key_3
  LIMIT sqlc.arg('batch')::int;

-- name: ExportAudit :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (a.seq::text)::text AS record_id,
         a.occurred_at AS changed_at,
         (to_jsonb(a) - 'tenant_id')::jsonb AS payload,
         a.seq AS key_1
  FROM audit_log a
) s
  WHERE (sqlc.narg('since')::timestamptz IS NULL OR s.changed_at > sqlc.narg('since')::timestamptz)
    AND (s.changed_at, s.key_1) > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_id')::bigint)
  ORDER BY s.changed_at, s.key_1
  LIMIT sqlc.arg('batch')::int;

-- One entity at a time, because each entity's deletions belong in its own data file - and because
-- the primary key starts (tenant_id, entity), so asking for one entity is an index range rather
-- than a scan. Paged on (deleted_at, entity_id) for the reason every statement above is: a page
-- that repeated or skipped a row would put a tombstone in one archive and nowhere else.
-- name: ExportTombstones :many
SELECT entity_id::text AS entity_id, deleted_at
FROM tombstone
WHERE entity = sqlc.arg('entity')::text
  AND deleted_at > sqlc.arg('since')::timestamptz
  AND (deleted_at, entity_id) > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_id')::uuid)
ORDER BY deleted_at, entity_id
LIMIT sqlc.arg('batch')::int;

-- Where the bytes of one medium lie, by the checksum the archive addresses it with.
--
-- READY only, and a checksum that is actually there: a PENDING upload is one whose bytes were
-- never read back and judged (C-06, migration 0013), and an object with no recorded checksum has
-- no content address at all. Neither is carried into an archive, and the row itself still is - so
-- a restore keeps the metadata and knows the bytes are gone.
-- name: FindMediaStorageKey :one
SELECT storage_key, byte_size
FROM media_object
WHERE checksum = sqlc.arg('checksum')::text
  AND status = 'READY'
  AND deleted_at IS NULL
ORDER BY created_at
LIMIT 1;
