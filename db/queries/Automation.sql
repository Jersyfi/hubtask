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
  trigger, conditions, actions, throttle, on_error, created_by, created_at, updated_at, version,
  next_run_at
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('scope_type'), sqlc.narg('scope_id'),
  sqlc.arg('name'), sqlc.arg('enabled'), sqlc.arg('run_as'),
  sqlc.arg('trigger'), sqlc.arg('conditions'), sqlc.arg('actions'), sqlc.arg('throttle'),
  sqlc.arg('on_error'), sqlc.arg('created_by'), sqlc.arg('created_at'), sqlc.arg('created_at'), 1,
  sqlc.narg('next_run_at')
);

-- name: FindAutomationRule :one
SELECT id, scope_type, scope_id, name, enabled, run_as, trigger, conditions, actions,
       throttle, on_error, failure_count, created_by, created_at, updated_at, deleted_at, version,
       next_run_at
FROM automation_rule
WHERE id = sqlc.arg('id') AND deleted_at IS NULL;

-- name: ListAutomationRules :many
-- Newest first by identifier: UUIDv7 is time-ordered, so the primary key is the creation order.
--
-- The `enabled` filter is a nullable argument rather than two queries: NULL means "either", which
-- is what an absent query parameter means, and a second statement differing in one predicate is a
-- second place for the `deleted_at` guard to be forgotten.
SELECT id, scope_type, scope_id, name, enabled, run_as, trigger, conditions, actions,
       throttle, on_error, failure_count, created_by, created_at, updated_at, deleted_at, version,
       next_run_at
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
    -- An edit may change the recurrence rule, so the moment is recomputed with the definition
    -- rather than left pointing at an occurrence of a rule that no longer exists.
    next_run_at = sqlc.narg('next_run_at'),
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

-- The run log (G-07, automation.md §2). What a rule did, why it did not, and what each action
-- answered - the record §2 promises is retrievable and filterable.

-- name: InsertRuleRun :exec
-- Written when the run starts, in the RUNNING state, before any condition is evaluated.
--
-- Before rather than after, because a row written when a run ends loses exactly the runs somebody
-- needs to see: a process that dies mid-run leaves RUNNING behind, and that is the only thing that
-- distinguishes a crash from a run nobody attempted.
INSERT INTO rule_run (
  id, tenant_id, rule_id, event_id, trigger, triggered_by, subject_id,
  status, condition_results, action_results, started_at, causation_depth
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('rule_id'), sqlc.narg('event_id'),
  sqlc.arg('trigger'), sqlc.narg('triggered_by'), sqlc.narg('subject_id'),
  sqlc.arg('status'), sqlc.arg('condition_results'), sqlc.arg('action_results'),
  sqlc.arg('started_at'), sqlc.arg('causation_depth')
);

-- name: FinishRuleRun :exec
-- The one statement that ends a run, whichever way it ended.
UPDATE rule_run
SET status            = sqlc.arg('status'),
    condition_results = sqlc.arg('condition_results'),
    action_results    = sqlc.arg('action_results'),
    error_code        = sqlc.narg('error_code'),
    finished_at       = sqlc.arg('finished_at')
WHERE id = sqlc.arg('id');

-- name: FindRuleRun :one
SELECT id, rule_id, event_id, trigger, triggered_by, subject_id, status,
       condition_results, action_results, error_code, started_at, finished_at, causation_depth
FROM rule_run
WHERE id = sqlc.arg('id');

-- name: ListRuleRuns :many
-- Newest first by identifier: UUIDv7 is time-ordered, so the primary key is the order runs happened
-- in. The three filters are nullable arguments rather than eight statements, because a second
-- statement differing in one predicate is a second place for a predicate to be forgotten.
SELECT id, rule_id, event_id, trigger, triggered_by, subject_id, status,
       condition_results, action_results, error_code, started_at, finished_at, causation_depth
FROM rule_run
WHERE (sqlc.narg('rule_id')::uuid IS NULL OR rule_id = sqlc.narg('rule_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('trigger')::text IS NULL OR trigger = sqlc.narg('trigger')::text)
  AND (sqlc.narg('after')::uuid IS NULL OR id < sqlc.narg('after')::uuid)
ORDER BY id DESC
LIMIT sqlc.arg('page_size');

-- name: CountRunsSince :one
-- What the throttle asks: how often this rule has run in the window (automation.md §2).
--
-- Counted from the run log rather than from a counter on the rule, and that is the point. A counter
-- would need resetting, and something has to decide when an hour has passed - which is a second
-- piece of bookkeeping that can be wrong in a way nobody sees. The log already knows, and a count
-- over an index is cheaper than a piece of state that can drift.
--
-- THROTTLED runs are not counted. A rule held back did not run, and counting the refusals would make
-- the bound tighten on itself: once a rule hit its limit it would stay there for the whole window
-- however quiet the workspace went.
SELECT count(*) FROM rule_run
WHERE rule_id = sqlc.arg('rule_id')
  AND started_at >= sqlc.arg('since')
  AND status <> 'THROTTLED';

-- name: BumpRuleFailure :one
-- One more consecutive failure, and the count afterwards.
--
-- Returned rather than read back, because the decision that follows it - has this rule failed often
-- enough to be switched off - has to be made on the value this statement produced. A read after the
-- write would be a second statement another run could commit between, and two runs failing together
-- would each see the other's count and neither would reach the threshold.
UPDATE automation_rule
SET failure_count = failure_count + 1,
    updated_at    = sqlc.arg('at')
WHERE id = sqlc.arg('id') AND deleted_at IS NULL
RETURNING failure_count;

-- name: ClearRuleFailure :exec
-- A run that worked ends the streak. `consecutive` is what the counter means, so one success
-- resets it rather than decrementing.
UPDATE automation_rule
SET failure_count = 0, updated_at = sqlc.arg('at')
WHERE id = sqlc.arg('id') AND deleted_at IS NULL AND failure_count <> 0;

-- name: DisableFailingRule :execrows
-- Switching a rule off because it kept failing.
--
-- Its own statement rather than SetAutomationRuleEnabled, and guarded on the count rather than on a
-- version. Nobody read this rule in order to switch it off - a run did, after failing - so there is
-- no version to have expected; what makes it safe is that it only fires while the count is still at
-- or past the threshold, so two runs failing together disable the rule once.
UPDATE automation_rule
SET enabled = false, updated_at = sqlc.arg('at'), version = version + 1
WHERE id = sqlc.arg('id') AND deleted_at IS NULL AND enabled = true
  AND failure_count >= sqlc.arg('threshold');

-- name: RulesForEventType :many
-- What the subscriber asks per event: the enabled rules whose trigger is this event type.
--
-- The event type is cast, because `->>` gives sqlc nothing to infer a parameter's type from:
-- without it the argument is generated as bytes rather than as the text the column holds.
--
-- The scope is not in the predicate. A rule scoped to a hub matches an event in that hub's
-- collections, and the event carries a subject rather than a path - so the narrowing is the
-- subscriber's, against what it can resolve, rather than a join this statement cannot make.
SELECT id, scope_type, scope_id, name, enabled, run_as, trigger, conditions, actions,
       throttle, on_error, failure_count, created_by, created_at, updated_at, deleted_at, version,
       next_run_at
FROM automation_rule
WHERE deleted_at IS NULL
  AND enabled = true
  AND trigger ->> 'kind' = 'EVENT'
  AND trigger ->> 'event_type' = sqlc.arg('event_type')::text
ORDER BY id;

-- The SCHEDULE trigger (G-08, decision 5 of milestone-0.5.0). The same three statements
-- `backup_schedule` has, and deliberately: this installation has one schedule engine, and a second
-- shape for reading a due moment would be a second engine in everything but name.

-- name: DueAutomationRules :many
-- What one pass claims: this tenant's rules whose moment has come, oldest first.
--
-- The tenant is row level security's, not a parameter (ADR-0010). `FOR UPDATE SKIP LOCKED` is
-- deliberately absent: the pass runs inside one tenant's own poller, of which there is one job, and
-- the poller holds a row lock on that job before it reads - so two passes for one tenant cannot
-- overlap, and a lock here would be a second answer to a question the queue has answered.
SELECT id, scope_type, scope_id, name, enabled, run_as, trigger, conditions, actions,
       throttle, on_error, failure_count, created_by, created_at, updated_at, deleted_at, version,
       next_run_at
FROM automation_rule
WHERE deleted_at IS NULL AND enabled = true
  AND next_run_at IS NOT NULL AND next_run_at <= sqlc.arg('due')
ORDER BY next_run_at
LIMIT sqlc.arg('page_size');

-- name: NextDueAutomationRule :one
-- The earliest moment this tenant owes anything, and NULL when it owes nothing - which is what
-- lets the poller finish instead of spinning. `max`/`min` over the partial index rather than a
-- LIMIT 1 with an ORDER BY, so an empty result is a row with NULL rather than no row at all.
SELECT min(next_run_at)::timestamptz AS next_run_at
FROM automation_rule
WHERE deleted_at IS NULL AND enabled = true AND next_run_at IS NOT NULL;

-- name: SetAutomationRuleNextRun :exec
-- Moves one rule on to its next moment.
--
-- The version is deliberately untouched. Nobody read this rule in order to advance it - a pass did,
-- because a moment arrived - and bumping the version would make every occurrence look like an edit
-- to a client holding an optimistic lock.
UPDATE automation_rule
SET next_run_at = sqlc.narg('next_run_at')
WHERE id = sqlc.arg('id') AND deleted_at IS NULL;
