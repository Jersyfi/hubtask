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
