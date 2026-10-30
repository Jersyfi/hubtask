-- The export the archive writer reads a tenant through (backup-restore.md §3, §5).
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
         -- A platform's reference is the provider's mapping, not the workspace's data: it stays
         -- out of the archive, and a copy restored beside the original cannot claim it.
         (to_jsonb(t) - 'tenant_id' - 'external_reference')::jsonb AS payload,
         t.id AS key_1
  FROM tenant t
) s
  WHERE (sqlc.narg('since')::timestamptz IS NULL OR s.changed_at > sqlc.narg('since')::timestamptz)
    AND (s.changed_at, s.key_1) > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_id')::uuid)
  ORDER BY s.changed_at, s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportAccounts :many
-- ai_consent is left out too: nothing reads or writes it, the person's AI objection is the consent
-- record, and an archive carrying an always-false "consent" would state one nobody gave
-- (tenant-export.md §11). The column itself goes in a later contract migration (rule 12).
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (a.id)::text AS record_id,
         a.updated_at AS changed_at,
         (to_jsonb(a) - 'tenant_id' - 'ai_consent')::jsonb AS payload,
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

-- name: ExportCustomRoles :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (r.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(r) - 'tenant_id')::jsonb AS payload,
         r.id AS key_1
  FROM custom_role r
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportCustomRoleGrants :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (g.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(g) - 'tenant_id')::jsonb AS payload,
         g.id AS key_1
  FROM custom_role_grant g
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportConfigRevisions :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (r.version::text)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(r) - 'tenant_id')::jsonb AS payload,
         r.version::bigint AS key_1
  FROM config_revision r
) s
  WHERE s.key_1 > sqlc.arg('after_id')::bigint
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportConfigProposals :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (p.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(p) - 'tenant_id')::jsonb AS payload,
         p.id AS key_1
  FROM config_proposal p
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportConfigTemplates :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (t.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(t) - 'tenant_id')::jsonb AS payload,
         t.id AS key_1
  FROM config_template t
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportConfigTemplateUses :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (u.template_key)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(u) - 'tenant_id')::jsonb AS payload,
         u.template_key AS key_1
  FROM config_template_use u
) s
  WHERE s.key_1 > sqlc.arg('after_id')::text
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

-- name: ExportPasskeys :many
-- Exported whole: a passkey changes only by its counter and its last use, and no column says when.
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM webauthn_credential x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportPrivateHubAccesses :many
-- Exported whole: an access is written once and never changes.
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM private_hub_access x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportContainerMilestones :many
-- Exported whole: a milestone goes with its container and leaves no deletion marker of its own.
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.container_id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.container_id AS key_1
  FROM container_milestone x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportStructurePresets :many
-- Exported whole: a withdrawn preset leaves no deletion marker of its own.
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (p.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(p) - 'tenant_id')::jsonb AS payload,
         p.id AS key_1
  FROM structure_preset p
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
-- never read back and judged (migration 0013), and an object with no recorded checksum has
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

-- name: ExportSettingValues :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (v.id)::text AS record_id,
         v.updated_at AS changed_at,
         (to_jsonb(v) - 'tenant_id')::jsonb AS payload,
         v.id AS key_1
  FROM setting_value v
) s
  WHERE (sqlc.narg('since')::timestamptz IS NULL OR s.changed_at > sqlc.narg('since')::timestamptz)
    AND (s.changed_at, s.key_1) > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_id')::uuid)
  ORDER BY s.changed_at, s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportEntryTypes :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (t.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(t) - 'tenant_id')::jsonb AS payload,
         t.id AS key_1
  FROM entry_type t
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportLinkTypes :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (t.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(t) - 'tenant_id')::jsonb AS payload,
         t.id AS key_1
  FROM link_type t
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportEntryLinks :many
-- The generated `own_type` stays out: the restore's insert derives it again.
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (l.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(l) - 'tenant_id' - 'own_type')::jsonb AS payload,
         l.id AS key_1
  FROM entry_link l
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportWorkflows :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (w.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(w) - 'tenant_id')::jsonb AS payload,
         w.id AS key_1
  FROM workflow w
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportWorkflowSchemes :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (w.container_id::text || '/' || w.entry_type)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(w) - 'tenant_id')::jsonb AS payload,
         w.container_id AS key_1,
         w.entry_type AS key_2
  FROM workflow_scheme w
) s
  WHERE (s.key_1, s.key_2) > (sqlc.arg('after_id')::uuid, sqlc.arg('after_second')::text)
  ORDER BY s.key_1, s.key_2
  LIMIT sqlc.arg('batch')::int;

-- name: ExportRealtimeDocuments :many
-- Live documents (migration 0230): the head, the log and the snapshots, written whole - an update
-- removed by retention leaves no marker.
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (d.document_id::text || '/' || d.kind)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(d) - 'tenant_id')::jsonb AS payload,
         d.document_id AS key_1,
         d.kind AS key_2
  FROM realtime_document d
) s
  WHERE (s.key_1, s.key_2) > (sqlc.arg('after_id')::uuid, sqlc.arg('after_second')::text)
  ORDER BY s.key_1, s.key_2
  LIMIT sqlc.arg('batch')::int;

-- name: ExportRealtimeUpdates :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (u.document_id::text || '/' || u.kind || '/' || u.seq::text)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(u) - 'tenant_id')::jsonb AS payload,
         u.document_id AS key_1,
         u.kind AS key_2,
         u.seq AS key_3
  FROM realtime_update u
) s
  WHERE (s.key_1, s.key_2, s.key_3) > (sqlc.arg('after_id')::uuid, sqlc.arg('after_second')::text, sqlc.arg('after_third')::bigint)
  ORDER BY s.key_1, s.key_2, s.key_3
  LIMIT sqlc.arg('batch')::int;

-- name: ExportRealtimeSnapshots :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (r.document_id::text || '/' || r.kind || '/' || r.up_to_seq::text)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(r) - 'tenant_id')::jsonb AS payload,
         r.document_id AS key_1,
         r.kind AS key_2,
         r.up_to_seq AS key_3
  FROM realtime_snapshot r
) s
  WHERE (s.key_1, s.key_2, s.key_3) > (sqlc.arg('after_id')::uuid, sqlc.arg('after_second')::text, sqlc.arg('after_third')::bigint)
  ORDER BY s.key_1, s.key_2, s.key_3
  LIMIT sqlc.arg('batch')::int;

-- name: ExportPages :many
-- The generated search document stays out: the restore's write builds it again.
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (p.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(p) - 'tenant_id' - 'search_document')::jsonb AS payload,
         p.id AS key_1
  FROM page p
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportPageVersions :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (v.page_id::text || '/' || v.seq::text)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(v) - 'tenant_id')::jsonb AS payload,
         v.page_id AS key_1,
         v.seq::bigint AS key_2
  FROM page_version v
) s
  WHERE (s.key_1, s.key_2) > (sqlc.arg('after_id')::uuid, sqlc.arg('after_second')::bigint)
  ORDER BY s.key_1, s.key_2
  LIMIT sqlc.arg('batch')::int;

-- name: ExportPageComments :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (c.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(c) - 'tenant_id')::jsonb AS payload,
         c.id AS key_1
  FROM page_comment c
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportWhiteboards :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (w.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(w) - 'tenant_id')::jsonb AS payload,
         w.id AS key_1
  FROM whiteboard w
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportWhiteboardVersions :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (v.whiteboard_id::text || '/' || v.seq::text)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(v) - 'tenant_id')::jsonb AS payload,
         v.whiteboard_id AS key_1,
         v.seq::bigint AS key_2
  FROM whiteboard_version v
) s
  WHERE (s.key_1, s.key_2) > (sqlc.arg('after_id')::uuid, sqlc.arg('after_second')::bigint)
  ORDER BY s.key_1, s.key_2
  LIMIT sqlc.arg('batch')::int;

-- name: ExportWhiteboardMedia :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (b.whiteboard_id::text || '/' || b.media_id::text)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(b) - 'tenant_id')::jsonb AS payload,
         b.whiteboard_id AS key_1,
         b.media_id AS key_2
  FROM whiteboard_media b
) s
  WHERE (s.key_1, s.key_2) > (sqlc.arg('after_id')::uuid, sqlc.arg('after_second')::uuid)
  ORDER BY s.key_1, s.key_2
  LIMIT sqlc.arg('batch')::int;


-- name: ExportCmdbClasses :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM cmdb_class x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportCmdbRelationTypes :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM cmdb_relation_type x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportCmdbSources :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM cmdb_source x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportCmdbCis :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM cmdb_ci x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportCmdbCiIdentifiers :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.ci_id::text || '/' || x.name)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.ci_id AS key_1,
         x.name::text AS key_2
  FROM cmdb_ci_identifier x
) s
  WHERE (s.key_1, s.key_2) > (sqlc.arg('after_id')::uuid, sqlc.arg('after_second')::text)
  ORDER BY s.key_1, s.key_2
  LIMIT sqlc.arg('batch')::int;

-- name: ExportCmdbRelations :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM cmdb_relation x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportCmdbRuns :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM cmdb_run x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportCmdbHolds :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM cmdb_hold x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportCmdbDuplicates :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM cmdb_duplicate x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportCmdbConnectors :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM cmdb_connector x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportCmdbAlerts :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM cmdb_alert x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportBusinessCalendars :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM business_calendar x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportSlaPolicies :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM sla_policy x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportEntrySlas :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM entry_sla x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportReplyTemplates :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM reply_template x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportSatisfactionRatings :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM satisfaction_rating x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportEntrySchedules :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.entry_id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.entry_id AS key_1
  FROM entry_schedule x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportPlanBaselines :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM plan_baseline x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportPlanBaselineEntries :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.baseline_id::text || '/' || x.entry_id::text)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.baseline_id AS key_1,
         x.entry_id AS key_2
  FROM plan_baseline_entry x
) s
  WHERE (s.key_1, s.key_2) > (sqlc.arg('after_id')::uuid, sqlc.arg('after_second')::uuid)
  ORDER BY s.key_1, s.key_2
  LIMIT sqlc.arg('batch')::int;

-- name: ExportSprints :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM sprint x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportSprintEntries :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.sprint_id::text || '/' || x.entry_id::text)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.sprint_id AS key_1,
         x.entry_id AS key_2
  FROM sprint_entry x
) s
  WHERE (s.key_1, s.key_2) > (sqlc.arg('after_id')::uuid, sqlc.arg('after_second')::uuid)
  ORDER BY s.key_1, s.key_2
  LIMIT sqlc.arg('batch')::int;

-- name: ExportEntryEstimates :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.entry_id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.entry_id AS key_1
  FROM entry_estimate x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportTimeWorkTypes :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM time_work_type x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportTimeRecords :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM time_record x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportTimeTimers :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM time_timer x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportTimesheets :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM timesheet x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportTimePolicies :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM time_policy x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportTimeCapacities :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM time_capacity x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportTimeAbsences :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM time_absence x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportTimeCostRates :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM time_cost_rate x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportTimeBudgets :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM time_budget x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportGoalPeriods :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM goal_period x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportGoals :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM goal x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportGoalEntries :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.goal_id::text || '/' || x.entry_id::text)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.goal_id AS key_1,
         x.entry_id AS key_2
  FROM goal_entry x
) s
  WHERE (s.key_1, s.key_2) > (sqlc.arg('after_id')::uuid, sqlc.arg('after_second')::uuid)
  ORDER BY s.key_1, s.key_2
  LIMIT sqlc.arg('batch')::int;

-- name: ExportGoalCheckIns :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM goal_check_in x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportPortfolios :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM portfolio x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportPortfolioItems :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.portfolio_id::text || '/' || x.container_id::text)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.portfolio_id AS key_1,
         x.container_id AS key_2
  FROM portfolio_item x
) s
  WHERE (s.key_1, s.key_2) > (sqlc.arg('after_id')::uuid, sqlc.arg('after_second')::uuid)
  ORDER BY s.key_1, s.key_2
  LIMIT sqlc.arg('batch')::int;

-- name: ExportPortfolioUpdates :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM portfolio_update x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportDashboards :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM dashboard x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportDashboardDigests :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM dashboard_digest x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportAgentModels :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM agent_model x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportAgents :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM agent x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportAgentHalts :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM agent_halt x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportAgentRequests :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM agent_request x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportAgentActions :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM agent_action x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportAgentEvalCases :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM agent_eval_case x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportAgentEvalRuns :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM agent_eval_run x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;

-- name: ExportAiProvenance :many
SELECT s.record_id, s.changed_at, s.payload
FROM (
  SELECT (x.id)::text::text AS record_id,
         NULL::timestamptz AS changed_at,
         (to_jsonb(x) - 'tenant_id')::jsonb AS payload,
         x.id AS key_1
  FROM ai_provenance x
) s
  WHERE s.key_1 > sqlc.arg('after_id')::uuid
  ORDER BY s.key_1
  LIMIT sqlc.arg('batch')::int;
