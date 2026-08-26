-- The rule model of data-retention.md §2 and the two phases that execute it (E-07).
--
-- Row level security supplies the tenant condition none of these statements writes (ADR-0010).
--
-- The anchor a period runs from is a column, and a column is not something a caller may name: rule
-- 9 admits no byte of a request into SQL text. So there is one candidate statement per anchor
-- rather than one statement with the column as a parameter, and the catalogue in the domain says
-- which of them a kind uses. Three anchors are enough for what this build sweeps; a fourth kind
-- brings a fourth statement, which is the honest cost of the rule.

-- name: InsertRetentionRule :exec
INSERT INTO retention_rule (
  id, tenant_id, scope_kind, scope_id, data_kind, condition, retain_days, action,
  then_after_days, then_action, grace_days, notify, justification, enabled, export_target_id,
  created_by, created_at, updated_at, version
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('scope_kind'), sqlc.narg('scope_id'),
  sqlc.arg('data_kind'), sqlc.narg('condition'), sqlc.arg('retain_days'), sqlc.arg('action'),
  sqlc.narg('then_after_days'), sqlc.narg('then_action'), sqlc.arg('grace_days'),
  sqlc.arg('notify'), sqlc.narg('justification'), sqlc.arg('enabled'),
  sqlc.narg('export_target_id'), sqlc.arg('created_by'), sqlc.arg('now'), sqlc.arg('now'), 1
);

-- name: ListRetentionRules :many
-- Every rule the tenant has, narrowest scope first so that a reader walking the list meets the
-- winner before the ones it beats.
SELECT id, scope_kind, scope_id, data_kind, condition, retain_days, action,
       then_after_days, then_action, grace_days, notify, justification, enabled,
       export_target_id, created_by, created_at, updated_at, version
FROM retention_rule
ORDER BY data_kind,
         CASE scope_kind WHEN 'COLLECTION' THEN 1 WHEN 'HUB' THEN 2 ELSE 3 END,
         created_at;

-- name: FindRetentionRule :one
SELECT id, scope_kind, scope_id, data_kind, condition, retain_days, action,
       then_after_days, then_action, grace_days, notify, justification, enabled,
       export_target_id, created_by, created_at, updated_at, version
FROM retention_rule
WHERE id = sqlc.arg('id');

-- name: CarryOverRetentionPolicy :exec
-- The old table's rows, written into the new one for a tenant that has no rule yet.
--
-- ON CONFLICT DO NOTHING, because this runs on every sweep and a tenant that has since written a
-- rule of its own has decided something - a carry-over that overwrote it would be the upgrade
-- quietly undoing the tenant's configuration. The scope is TENANT because that is what the old key
-- could express, and the action is HARD_DELETE because that is what the old engine did.
INSERT INTO retention_rule (
  id, tenant_id, scope_kind, scope_id, data_kind, retain_days, action, grace_days, notify,
  enabled, created_at, updated_at
)
SELECT sqlc.arg('id'), current_tenant_id(), 'TENANT', NULL, p.data_kind, p.retain_days,
       'HARD_DELETE', 0, '{}'::jsonb, true, sqlc.arg('now'), sqlc.arg('now')
FROM retention_policy p
WHERE p.data_kind = sqlc.arg('data_kind')
ON CONFLICT DO NOTHING;

-- The candidates of phase one, one statement per anchor.
--
-- Only what is not already marked and not already gone: a row the last pass marked is phase two's
-- business, and a row in the trash is the trash's. The hub is joined in because the scope of a rule
-- can be a hub and an entry three levels down does not know which hub it is under.

-- name: RetentionCandidatesByCompletedAt :many
SELECT w.id, w.type, w.path, w.collection_id, c.parent_id AS hub_id,
       w.completed_at AS anchored_at, w.title
FROM work_item w
JOIN container c ON c.id = w.collection_id
WHERE w.completed_at IS NOT NULL
  AND w.completed_at < sqlc.arg('cutoff')::timestamptz
  AND w.deleted_at IS NULL
  AND w.archived_at IS NULL
  AND w.retention_pending_until IS NULL
ORDER BY w.completed_at, w.id
LIMIT sqlc.arg('batch')::int;

-- name: RetentionCandidatesByArchivedAt :many
SELECT w.id, w.type, w.path, w.collection_id, c.parent_id AS hub_id,
       w.archived_at AS anchored_at, w.title
FROM work_item w
JOIN container c ON c.id = w.collection_id
WHERE w.archived_at IS NOT NULL
  AND w.archived_at < sqlc.arg('cutoff')::timestamptz
  AND w.deleted_at IS NULL
  AND w.retention_pending_until IS NULL
  -- A chain's second stage takes only what its own first stage acted on, which is what
  -- `retention_rule_id` survives the act for: an entry somebody archived by hand is not part of
  -- anybody's chain, and a rule that swept it up would be a rule acting outside what it matched.
  AND (NOT sqlc.arg('own_chain')::boolean OR w.retention_rule_id = sqlc.arg('rule_id')::uuid)
ORDER BY w.archived_at, w.id
LIMIT sqlc.arg('batch')::int;

-- name: RetentionCandidatesByDeletedAt :many
-- The trash: what the trash's own rule governs, and what a chain whose first stage put it there
-- comes back for.
SELECT w.id, w.type, w.path, w.collection_id, c.parent_id AS hub_id,
       w.deleted_at AS anchored_at, w.title
FROM work_item w
JOIN container c ON c.id = w.collection_id
WHERE w.deleted_at IS NOT NULL
  AND w.deleted_at < sqlc.arg('cutoff')::timestamptz
  AND w.retention_pending_until IS NULL
  AND (NOT sqlc.arg('own_chain')::boolean OR w.retention_rule_id = sqlc.arg('rule_id')::uuid)
ORDER BY w.deleted_at, w.id
LIMIT sqlc.arg('batch')::int;

-- name: MarkItemsForRetention :execrows
-- Phase one: what is coming, when, and under which rule (§5, §6).
--
-- The block is cleared with the marking: an entry that was held back and is now announced is an
-- entry whose obstacle has gone, and a stale reason on it would tell somebody a hold is in force
-- that was lifted last week.
UPDATE work_item SET
  retention_pending_until = sqlc.arg('effective_at')::timestamptz,
  retention_rule_id       = sqlc.arg('rule_id'),
  retention_action        = sqlc.arg('action'),
  retention_blocked_by    = NULL
WHERE id = ANY(sqlc.arg('ids')::uuid[])
  AND retention_pending_until IS NULL;

-- name: BlockItemsForRetention :execrows
-- What a rule would do, and what is stopping it (§4, §6).
--
-- No `retention_pending_until`, deliberately: an entry that is held back has no due moment, and the
-- absence of one is what keeps phase two off it rather than a flag somebody has to remember to
-- check. Re-run on every pass, because a block is a fact about now - the hold may have been lifted
-- since, and then the marking above takes over.
UPDATE work_item SET
  retention_rule_id    = sqlc.arg('rule_id'),
  retention_action     = sqlc.arg('action'),
  retention_blocked_by = sqlc.arg('blocked_by')
WHERE id = ANY(sqlc.arg('ids')::uuid[])
  AND retention_pending_until IS NULL;

-- name: RetentionMarkedItemsDue :many
-- Phase two: what the grace period has run out on.
SELECT w.id, w.type, w.path, w.collection_id, c.parent_id AS hub_id,
       w.retention_pending_until, w.retention_rule_id, w.retention_action, w.title
FROM work_item w
JOIN container c ON c.id = w.collection_id
WHERE w.retention_pending_until IS NOT NULL
  AND w.retention_pending_until <= sqlc.arg('now')::timestamptz
ORDER BY w.retention_pending_until, w.id
LIMIT sqlc.arg('batch')::int;

-- name: FindItemRetention :one
-- What one entry's marking says, which is what `:retain` reads and what the object carries.
SELECT retention_pending_until, retention_rule_id, retention_action, retention_blocked_by
FROM work_item
WHERE id = sqlc.arg('id') AND deleted_at IS NULL;

-- name: ClearItemRetention :execrows
-- `:retain`, and the end of a stage. The rule reference is cleared with it when a person takes the
-- entry out - taking it out means the rule no longer owns it - and kept when a stage has acted,
-- because the chain's next stage is what owns it then.
UPDATE work_item SET
  retention_pending_until = NULL,
  retention_action        = NULL,
  retention_blocked_by    = NULL,
  retention_rule_id       = CASE WHEN sqlc.arg('keep_rule')::boolean THEN retention_rule_id END,
  updated_at              = sqlc.arg('now')::timestamptz,
  version                 = version + 1
WHERE id = ANY(sqlc.arg('ids')::uuid[])
  AND retention_pending_until IS NOT NULL;

-- name: ArchiveItemsForRetention :execrows
-- The act of an ARCHIVE stage. Only what is not archived already, so a repeated pass writes the
-- same rows rather than moving the date somebody will later count a second stage from.
UPDATE work_item SET
  archived_at = sqlc.arg('at')::timestamptz,
  updated_at  = sqlc.arg('at')::timestamptz,
  version     = version + 1
WHERE id = ANY(sqlc.arg('ids')::uuid[])
  AND archived_at IS NULL
  AND deleted_at IS NULL;

-- name: TrashItemsForRetention :execrows
-- The act of a TRASH stage. The batch identifier is what makes one act one restore (F-09).
UPDATE work_item SET
  deleted_at     = sqlc.arg('at')::timestamptz,
  trash_batch_id = sqlc.arg('batch_id'),
  updated_at     = sqlc.arg('at')::timestamptz,
  version        = version + 1
WHERE id = ANY(sqlc.arg('ids')::uuid[])
  AND deleted_at IS NULL;

-- name: CountRetainedDescendants :many
-- §4.6, the referential safeguard: a work package is not removed while something below it is kept
-- for longer. Answered for a batch rather than per row, because a pass judges a thousand at a time.
--
-- "Below it" is the path prefix, and "kept for longer" is anything under that prefix which is not
-- itself due in this pass - which the caller decides by handing in the identities it has already
-- judged.
SELECT parent.id, count(child.id)::bigint AS retained
FROM work_item parent
JOIN work_item child
  -- Every path ends with the separator, so a prefix test needs no separator of its own - and the
  -- parent matches its own prefix, which is what the identity comparison excludes.
  ON child.path LIKE parent.path || '%'
 AND child.id <> parent.id
 AND child.deleted_at IS NULL
 AND NOT (child.id = ANY(sqlc.arg('going')::uuid[]))
WHERE parent.id = ANY(sqlc.arg('ids')::uuid[])
GROUP BY parent.id;

-- name: CountRetentionScope :one
-- The denominator of the five-per-cent switch: how much the tenant holds, in the scope the rule
-- covers. Live entries only - counting the trash would make a rule about completed work look
-- narrower the fuller the trash was.
SELECT count(*)::bigint
FROM work_item w
JOIN container c ON c.id = w.collection_id
WHERE w.deleted_at IS NULL
  AND (sqlc.arg('scope_kind')::text = 'TENANT'
       OR (sqlc.arg('scope_kind')::text = 'HUB' AND c.parent_id = sqlc.arg('scope_id')::uuid)
       OR (sqlc.arg('scope_kind')::text = 'COLLECTION' AND w.collection_id = sqlc.arg('scope_id')::uuid));

-- The numerator of the five-per-cent switch and of a preview, per anchor.
--
-- Exact rather than bounded, because §5's switch is about a proportion and a count that stopped at a
-- batch would under-report exactly the runs the switch exists to catch. Within the rule's scope and
-- ignoring narrower rules, which over-counts where one exists - and over-counting errs towards
-- NOTIFY_ONLY, which is the side to err on.

-- name: CountRetentionCandidatesByCompletedAt :one
SELECT count(*)::bigint
FROM work_item w
JOIN container c ON c.id = w.collection_id
WHERE w.completed_at IS NOT NULL
  AND w.completed_at < sqlc.arg('cutoff')::timestamptz
  AND w.deleted_at IS NULL
  AND w.archived_at IS NULL
  AND (sqlc.arg('scope_kind')::text = 'TENANT'
       OR (sqlc.arg('scope_kind')::text = 'HUB' AND c.parent_id = sqlc.arg('scope_id')::uuid)
       OR (sqlc.arg('scope_kind')::text = 'COLLECTION' AND w.collection_id = sqlc.arg('scope_id')::uuid));

-- name: CountRetentionCandidatesByArchivedAt :one
SELECT count(*)::bigint
FROM work_item w
JOIN container c ON c.id = w.collection_id
WHERE w.archived_at IS NOT NULL
  AND w.archived_at < sqlc.arg('cutoff')::timestamptz
  AND w.deleted_at IS NULL
  AND (sqlc.arg('scope_kind')::text = 'TENANT'
       OR (sqlc.arg('scope_kind')::text = 'HUB' AND c.parent_id = sqlc.arg('scope_id')::uuid)
       OR (sqlc.arg('scope_kind')::text = 'COLLECTION' AND w.collection_id = sqlc.arg('scope_id')::uuid));

-- name: CountRetentionCandidatesByDeletedAt :one
SELECT count(*)::bigint
FROM work_item w
JOIN container c ON c.id = w.collection_id
WHERE w.deleted_at IS NOT NULL
  AND w.deleted_at < sqlc.arg('cutoff')::timestamptz
  AND (sqlc.arg('scope_kind')::text = 'TENANT'
       OR (sqlc.arg('scope_kind')::text = 'HUB' AND c.parent_id = sqlc.arg('scope_id')::uuid)
       OR (sqlc.arg('scope_kind')::text = 'COLLECTION' AND w.collection_id = sqlc.arg('scope_id')::uuid));
