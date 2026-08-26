-- Data subject rights (E-10, data-protection.md §4). The tables have stood since `0001_init`;
-- these are the first statements over them.
--
-- The tenant is never a parameter. It comes from the transaction's own context through
-- current_tenant_id(), which is the value row level security compares against (ADR-0010) - and
-- that holds for the installation-wide case as well: it opens one transaction per tenant rather
-- than one query across tenants.

-- name: InsertDataSubjectRequest :exec
INSERT INTO data_subject_request (
  id, tenant_id, subject_account_id, subject_email, kind, status, scope,
  erasure_mode, received_at, due_at, handled_by, target_id, notes
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.narg('subject_account_id'), sqlc.narg('subject_email'),
  sqlc.arg('kind'), sqlc.arg('status'), sqlc.arg('scope'),
  sqlc.narg('erasure_mode'), sqlc.arg('received_at'), sqlc.arg('due_at'),
  sqlc.narg('handled_by'), sqlc.narg('target_id'), sqlc.narg('notes')
);

-- name: FindDataSubjectRequest :one
SELECT id, subject_account_id, subject_email, kind, status, scope, erasure_mode,
       received_at, due_at, completed_at, handled_by, rejection_reason,
       target_id, result_archive, notes
FROM data_subject_request
WHERE id = sqlc.arg('id') AND tenant_id = current_tenant_id();

-- name: UpdateDataSubjectRequest :execrows
-- The whole case as the domain decided it, written back in one statement.
--
-- Every column the state machine can move is here and no others: a case carries a person's request
-- and the answer to it, and an update that could write `received_at` or the subject would be one
-- that can rewrite what somebody asked for.
UPDATE data_subject_request SET
  status           = sqlc.arg('status'),
  erasure_mode     = sqlc.narg('erasure_mode'),
  handled_by       = sqlc.narg('handled_by'),
  rejection_reason = sqlc.narg('rejection_reason'),
  completed_at     = sqlc.narg('completed_at'),
  target_id        = sqlc.narg('target_id'),
  result_archive   = sqlc.narg('result_archive'),
  subject_account_id = sqlc.narg('subject_account_id'),
  notes            = sqlc.narg('notes')
WHERE id = sqlc.arg('id') AND tenant_id = current_tenant_id();

-- name: ListDataSubjectRequests :many
-- One page of the cases, the soonest deadline first.
--
-- By deadline rather than by receipt, because that is the order the work has to be done in: a case
-- recorded yesterday with a week to run is more urgent than one recorded last month with three.
-- The open ones alone unless the caller asks for the closed ones too - "what do we owe" is what
-- this list answers.
--
-- Served by dsr_open_idx (tenant_id, status, due_at) for the open half, which is the half that is
-- read.
SELECT id, subject_account_id, subject_email, kind, status, scope, erasure_mode,
       received_at, due_at, completed_at, handled_by, rejection_reason,
       target_id, result_archive, notes
FROM data_subject_request
WHERE tenant_id = current_tenant_id()
  AND (sqlc.arg('include_closed')::boolean OR status IN ('RECEIVED','IN_PROGRESS'))
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('kind')::text IS NULL OR kind = sqlc.narg('kind')::text)
  AND (sqlc.narg('due_before')::timestamptz IS NULL OR due_at < sqlc.narg('due_before')::timestamptz)
  AND (
    sqlc.narg('cursor_due_at')::timestamptz IS NULL
    OR (due_at, id) > (sqlc.narg('cursor_due_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY due_at ASC, id ASC
LIMIT sqlc.arg('page_size');

-- name: OverdueRequestCount :one
-- How many open cases are past their deadline, and how soon the next one falls due.
--
-- The reading behind alert A-19 (data-protection.md §4): "without deadline monitoring, the right
-- gets violated in practice even though the feature exists". Two numbers rather than a list,
-- because a gauge is what an alert evaluates and the list is a page somebody reads.
SELECT
  count(*) FILTER (WHERE due_at < sqlc.arg('now')::timestamptz) AS overdue,
  count(*) AS open_cases,
  min(due_at) AS next_due_at
FROM data_subject_request
WHERE tenant_id = current_tenant_id()
  AND status IN ('RECEIVED','IN_PROGRESS');

-- name: InsertConsentRecord :exec
-- A consent record is never updated in place: what an operator has to be able to show is not "is
-- this allowed now" but "was it allowed then", so a change is a new row and the old one keeps the
-- period it covered.
INSERT INTO consent_record (id, tenant_id, account_id, purpose, granted, granted_at, revoked_at, source)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.narg('account_id'), sqlc.arg('purpose'),
  sqlc.arg('granted'), sqlc.arg('granted_at'), sqlc.narg('revoked_at'), sqlc.narg('source')
);

-- name: RevokeConsent :execrows
-- Ends every standing consent of one account for one purpose.
--
-- The moment is only written where there is none: when the processing stopped is the fact being
-- kept, and a second withdrawal must not move it.
UPDATE consent_record SET revoked_at = sqlc.arg('revoked_at')
WHERE tenant_id = current_tenant_id()
  AND purpose = sqlc.arg('purpose')
  AND account_id IS NOT DISTINCT FROM sqlc.narg('account_id')
  AND granted
  AND revoked_at IS NULL;

-- name: LatestConsent :one
SELECT id, account_id, purpose, granted, granted_at, revoked_at, source
FROM consent_record
WHERE tenant_id = current_tenant_id()
  AND purpose = sqlc.arg('purpose')
  AND account_id IS NOT DISTINCT FROM sqlc.narg('account_id')
ORDER BY granted_at DESC
LIMIT 1;

-- name: SetAccountStatus :execrows
-- One column, for the reason UpdateAccountPreferences writes three: the status decides whether an
-- account may act at all, and a statement that could write anything else while setting it is one
-- that can change what somebody may do by accident.
UPDATE account SET status = sqlc.arg('status'), updated_at = sqlc.arg('updated_at'), version = version + 1
WHERE id = sqlc.arg('id') AND tenant_id = current_tenant_id() AND deleted_at IS NULL;

-- name: InsertAuditPseudonym :exec
-- The substitution the audit trail reads at the boundary (audit.md §6). Idempotent on the actor:
-- an erasure that is retried records the same pseudonym rather than a second one.
INSERT INTO audit_pseudonym (tenant_id, actor_id, pseudonym, reason, created_at)
VALUES (current_tenant_id(), sqlc.arg('actor_id'), sqlc.arg('pseudonym'), sqlc.arg('reason'), sqlc.arg('created_at'))
ON CONFLICT (tenant_id, actor_id) DO NOTHING;

-- name: AuditPseudonyms :many
-- The pseudonyms of a set of actors, for a page of the trail that has just been read.
--
-- Asked for the actors on the page rather than read whole: a workspace that has answered a hundred
-- erasures has a hundred rows here, and a page has at most a few dozen distinct actors.
SELECT actor_id, pseudonym
FROM audit_pseudonym
WHERE tenant_id = current_tenant_id() AND actor_id = ANY(sqlc.arg('actor_ids')::uuid[]);

-- name: SubjectTenants :many
-- The workspaces of this installation in which one person is a member.
--
-- The one cross-tenant question in the system (data-protection.md §4), answered by a function that
-- returns tenant identifiers and nothing else - never a name, never a row of anybody's data. What
-- follows is one ordinary transaction per tenant, under that tenant's own context
-- (db/migrations/0044_privacy_requests.sql).
SELECT subject_tenants AS tenant_id FROM subject_tenants(sqlc.arg('subject_email')::text);
