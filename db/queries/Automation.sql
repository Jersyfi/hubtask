-- The automation rules (G-05, automation.md §1). The table has carried the whole model since
-- 0001_init; these are the statements that finally write to it.
--
-- The tenant is never a parameter: row level security bounds every statement to the tenant of the
-- running transaction, which is what makes another workspace's rule invisible rather than forbidden
-- (ADR-0010, multi-tenancy.md §2).
--
-- `deleted_at IS NULL` is on every read. The deletion is soft so that the runs a rule produced stay
-- readable - a run log whose rule vanished would be a record of actions nobody can account for -
-- and a deleted rule is nevertheless gone from every question anybody asks about rules.

-- name: InsertAutomationRule :exec
-- A rule is written switched off, which is why `enabled` is a parameter rather than the column's
-- default: writing what a rule would do and letting it loose are two decisions, and the default
-- says the opposite.
INSERT INTO automation_rule (
  id, tenant_id, scope_type, scope_id, name, enabled, run_as,
  trigger, conditions, actions, throttle, on_error, created_by, created_at, updated_at, version
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('scope_type'), sqlc.narg('scope_id'),
  sqlc.arg('name'), sqlc.arg('enabled'), sqlc.arg('run_as'),
  sqlc.arg('trigger'), sqlc.arg('conditions'), sqlc.arg('actions'), sqlc.arg('throttle'),
  sqlc.arg('on_error'), sqlc.arg('created_by'), sqlc.arg('created_at'), sqlc.arg('created_at'), 1
);

-- name: FindAutomationRule :one
SELECT id, scope_type, scope_id, name, enabled, run_as, trigger, conditions, actions,
       throttle, on_error, failure_count, created_by, created_at, updated_at, deleted_at, version
FROM automation_rule
WHERE id = sqlc.arg('id') AND deleted_at IS NULL;

-- name: ListAutomationRules :many
-- Newest first by identifier: UUIDv7 is time-ordered, so the primary key is the creation order.
--
-- The `enabled` filter is a nullable argument rather than two queries: NULL means "either", which
-- is what an absent query parameter means, and a second statement differing in one predicate is a
-- second place for the `deleted_at` guard to be forgotten.
SELECT id, scope_type, scope_id, name, enabled, run_as, trigger, conditions, actions,
       throttle, on_error, failure_count, created_by, created_at, updated_at, deleted_at, version
FROM automation_rule
WHERE deleted_at IS NULL
  AND (sqlc.narg('enabled')::boolean IS NULL OR enabled = sqlc.narg('enabled')::boolean)
  AND (sqlc.narg('after')::uuid IS NULL OR id < sqlc.narg('after')::uuid)
ORDER BY id DESC
LIMIT sqlc.arg('page_size');

-- name: UpdateAutomationRule :execrows
-- The whole definition at once, and the version guard in the WHERE.
--
-- The guard is here rather than in a read-then-write, because a check in the application layer is a
-- check something else can commit between: two writers that both read version 3 would both find it
-- current and the second would overwrite the first. `execrows` returning zero is what "somebody got
-- there first" looks like from a statement that cannot be raced.
UPDATE automation_rule
SET scope_type = sqlc.arg('scope_type'),
    scope_id   = sqlc.narg('scope_id'),
    name       = sqlc.arg('name'),
    run_as     = sqlc.arg('run_as'),
    trigger    = sqlc.arg('trigger'),
    conditions = sqlc.arg('conditions'),
    actions    = sqlc.arg('actions'),
    throttle   = sqlc.arg('throttle'),
    on_error   = sqlc.arg('on_error'),
    updated_at = sqlc.arg('updated_at'),
    version    = version + 1
WHERE id = sqlc.arg('id') AND deleted_at IS NULL AND version = sqlc.arg('expected_version');

-- name: SetAutomationRuleEnabled :execrows
-- Switching a rule on or off, and nothing else.
--
-- Its own statement rather than a column in the update above, because the two are different acts
-- with different audit entries - and one statement carrying both would let an edit switch a rule on
-- as a side effect of changing its name.
--
-- The failure counter is cleared only on the way on. A rule somebody has looked at and turned back
-- on is one whose run of failures has been dealt with; a rule switched off by hand has not been
-- fixed, and clearing the count there would forget why it stopped.
UPDATE automation_rule
SET enabled       = sqlc.arg('enabled'),
    failure_count = CASE WHEN sqlc.arg('enabled')::boolean THEN 0 ELSE failure_count END,
    updated_at    = sqlc.arg('updated_at'),
    version       = version + 1
WHERE id = sqlc.arg('id') AND deleted_at IS NULL AND version = sqlc.arg('expected_version');

-- name: SoftDeleteAutomationRule :execrows
-- The rule stops matching anything at once and leaves the listing; its runs stay readable.
--
-- Also switched off, rather than left enabled behind the tombstone. Every read here filters on
-- `deleted_at IS NULL` so nothing would find it either way, but a row that says `enabled = true`
-- and is never enabled again is a row that reads as a lie to whoever opens the table next.
UPDATE automation_rule
SET deleted_at = sqlc.arg('deleted_at'),
    enabled    = false,
    updated_at = sqlc.arg('deleted_at'),
    version    = version + 1
WHERE id = sqlc.arg('id') AND deleted_at IS NULL;
