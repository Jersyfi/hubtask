-- The series beside the entries (D-04, domain-model.md §3.5).
--
-- The tenant is never a parameter here: it comes from the transaction's own context through
-- current_tenant_id(), which is the same value row level security compares against (ADR-0010).
--
-- Nothing in this file expands anything. What a rule means is a library's answer behind a port
-- (ADR-0008), and what is stored is the text somebody wrote plus the fields that qualify it.

-- name: InsertRecurrenceRule :exec
INSERT INTO recurrence_rule (
  id, tenant_id, source_item_id, rrule, time_zone, mode, horizon_days, ends_at, max_count,
  created_at, version
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('source_item_id'), sqlc.arg('rrule'),
  sqlc.arg('time_zone'), sqlc.arg('mode'), sqlc.arg('horizon_days'), sqlc.narg('ends_at'),
  sqlc.narg('max_count'), sqlc.arg('created_at'), 1
);

-- name: FindRecurrenceRuleForItem :one
-- Every use case starts from the entry - the route is the entry's - so this is the only read the
-- rule needs. Served by recurrence_rule_source_idx, which is also what keeps it single.
SELECT id, tenant_id, source_item_id, rrule, time_zone, mode, horizon_days, ends_at, max_count,
       last_materialized_at, created_at, updated_at, version
FROM recurrence_rule
WHERE source_item_id = sqlc.arg('source_item_id');

-- name: UpdateRecurrenceRule :execrows
-- The whole document, under the same optimistic lock every other row takes (api-guidelines.md §5).
-- A series is one statement rather than six settings, which is why there is no per-column writer:
-- "every Monday in Berlin, on completion, ninety days ahead" is a sentence, and half of it is a
-- different one.
--
-- last_materialized_at is deliberately untouched. It is the materialisation's own bookkeeping
-- (D-05), and a rule that changed does not un-create the occurrences that already exist.
UPDATE recurrence_rule SET
  rrule        = sqlc.arg('rrule'),
  time_zone    = sqlc.arg('time_zone'),
  mode         = sqlc.arg('mode'),
  horizon_days = sqlc.arg('horizon_days'),
  ends_at      = sqlc.narg('ends_at'),
  max_count    = sqlc.narg('max_count'),
  updated_at   = sqlc.arg('updated_at'),
  version      = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: DeleteRecurrenceRule :execrows
-- A hard delete of one row. The occurrences it produced are ordinary entries and are not touched:
-- deleting somebody's next three weeks because they stopped a series would be a deletion nobody
-- asked for (D-04).
DELETE FROM recurrence_rule
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: SetWorkItemRecurrence :execrows
-- The entry's pointer at its series, set when a rule is stored and cleared when it goes.
--
-- No version and no stamp: the pointer is derived from whether a rule exists, so nobody edited the
-- entry - and a version spent here would answer a client's If-Match with a conflict about a field
-- it does not own. What serialises two writers is the rule's own lock, in whose transaction this
-- runs.
UPDATE work_item SET recurrence_rule_id = sqlc.narg('recurrence_rule_id')
WHERE id = sqlc.arg('id')::uuid;

-- name: ClaimRulesToMaterialize :many
-- What the materialisation pass takes: this tenant's series whose rolling window may owe
-- something, oldest bookkeeping first (D-05).
--
-- The predicate is the window itself: a rule whose watermark already reaches past the horizon owes
-- nothing until time moves. A rule that has never materialised has no watermark and is always a
-- candidate, which is what makes the first pass after a rule is written do the work.
--
-- FOR UPDATE SKIP LOCKED for the reason the job queue uses it (ADR-0008): two passes that overlap
-- take disjoint sets instead of waiting for each other. The second lock is the watermark's own
-- compare-and-set below, which is what makes a leader failover harmless even when they do meet.
SELECT id, tenant_id, source_item_id, rrule, time_zone, mode, horizon_days, ends_at, max_count,
       last_materialized_at, created_at, updated_at, version
FROM recurrence_rule
WHERE last_materialized_at IS NULL
   OR last_materialized_at < (sqlc.arg('now')::timestamptz + make_interval(days => horizon_days))
ORDER BY coalesce(last_materialized_at, created_at), id
LIMIT sqlc.arg('batch_size')
FOR UPDATE SKIP LOCKED;

-- name: AdvanceRecurrenceWatermark :execrows
-- How far the series has been materialised, moved forward under a compare-and-set.
--
-- The predicate is the whole exactly-once argument for occurrences. Two passes that read the same
-- watermark both create the same morning; the first to commit moves it, and the second matches no
-- row, fails, and rolls back the entries it wrote with it - because the pass and its bookkeeping
-- are one transaction. IS NOT DISTINCT FROM rather than =, because the first pass compares against
-- NULL and NULL = NULL is not true.
--
-- No version and no stamp: this is the materialisation's own bookkeeping, not an edit somebody
-- made, and a version spent here would answer a client's If-Match with a conflict about a field it
-- does not own.
UPDATE recurrence_rule SET last_materialized_at = sqlc.arg('materialized_at')
WHERE id = sqlc.arg('id')::uuid
  AND last_materialized_at IS NOT DISTINCT FROM sqlc.narg('expected');

-- name: CountOpenOccurrences :one
-- What an ON_COMPLETION series is waiting for: an entry of the series that is still open. The
-- template counts, which is what makes the first follow-up wait for the template's own completion
-- (arc42 §6.3). Trashed and archived entries do not: they are not going to be completed.
SELECT count(*) FROM work_item
WHERE recurrence_rule_id = sqlc.arg('recurrence_rule_id')
  AND is_completed = false AND deleted_at IS NULL AND archived_at IS NULL;

-- name: LatestOccurrenceCompletion :one
-- When the series was last done, which is where an ON_COMPLETION series counts its next occurrence
-- from: "again, two weeks after I last did it". NULL for a series nobody has completed yet.
SELECT max(completed_at)::timestamptz AS completed_at
FROM work_item
WHERE recurrence_rule_id = sqlc.arg('recurrence_rule_id');
