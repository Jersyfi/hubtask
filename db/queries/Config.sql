-- The configuration document's versions, proposals and templates (migration 0221, F-6). The tenant
-- is the transaction's in every statement (ADR-0010).

-- name: LatestConfigRevision :one
SELECT tenant_id, version, document, hash, plan_hash, actor_kind, actor_id, proposed_by,
       rolled_back_to, template, added, changed, removed, created_at
FROM config_revision
ORDER BY version DESC
LIMIT 1;

-- name: FindConfigRevision :one
SELECT tenant_id, version, document, hash, plan_hash, actor_kind, actor_id, proposed_by,
       rolled_back_to, template, added, changed, removed, created_at
FROM config_revision WHERE version = sqlc.arg('version');

-- name: ListConfigRevisions :many
-- Newest first, without the documents: a history lists, a comparison reads two versions.
SELECT tenant_id, version, hash, plan_hash, actor_kind, actor_id, proposed_by,
       rolled_back_to, template, added, changed, removed, created_at
FROM config_revision
WHERE sqlc.narg('before')::integer IS NULL OR version < sqlc.narg('before')::integer
ORDER BY version DESC
LIMIT sqlc.arg('max_rows');

-- name: InsertConfigRevision :exec
-- The version is the next of the workspace's; the primary key refuses two applies taking one.
INSERT INTO config_revision (tenant_id, version, document, hash, plan_hash, actor_kind, actor_id,
                             proposed_by, rolled_back_to, template, added, changed, removed, created_at)
VALUES (current_tenant_id(), sqlc.arg('version'), sqlc.arg('document'), sqlc.arg('hash'),
        sqlc.arg('plan_hash'), sqlc.arg('actor_kind'), sqlc.narg('actor_id'), sqlc.narg('proposed_by'),
        sqlc.narg('rolled_back_to'), sqlc.narg('template'), sqlc.arg('added'), sqlc.arg('changed'),
        sqlc.arg('removed'), sqlc.arg('at'));

-- name: InsertConfigProposal :exec
INSERT INTO config_proposal (id, tenant_id, state, document, mode, base_hash, plan_hash, template,
                             added, changed, removed, hides_data, changes_permissions, proposed_by,
                             created_at)
VALUES (sqlc.arg('id'), current_tenant_id(), 'WAITING', sqlc.arg('document'), sqlc.arg('mode'),
        sqlc.arg('base_hash'), sqlc.arg('plan_hash'), sqlc.narg('template'), sqlc.arg('added'),
        sqlc.arg('changed'), sqlc.arg('removed'), sqlc.arg('hides_data'),
        sqlc.arg('changes_permissions'), sqlc.narg('proposed_by'), sqlc.arg('at'));

-- name: FindConfigProposal :one
SELECT id, tenant_id, state, document, mode, base_hash, plan_hash, template, added, changed, removed,
       hides_data, changes_permissions, proposed_by, decided_by, created_at, decided_at
FROM config_proposal WHERE id = sqlc.arg('id');

-- name: ListConfigProposals :many
SELECT id, tenant_id, state, document, mode, base_hash, plan_hash, template, added, changed, removed,
       hides_data, changes_permissions, proposed_by, decided_by, created_at, decided_at
FROM config_proposal
WHERE sqlc.narg('state')::text IS NULL OR state = sqlc.narg('state')::text
ORDER BY created_at DESC
LIMIT sqlc.arg('max_rows');

-- name: DecideConfigProposal :execrows
-- Only a waiting proposal is decided, once: two people deciding at once do not both win.
UPDATE config_proposal
SET state = sqlc.arg('state'), decided_by = sqlc.narg('decided_by'), decided_at = sqlc.arg('at')
WHERE id = sqlc.arg('id') AND state = 'WAITING';

-- name: InsertConfigTemplate :exec
INSERT INTO config_template (id, tenant_id, key, version, name, origin, document, created_by, created_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('key'), sqlc.arg('version'), sqlc.arg('name'),
        sqlc.arg('origin'), sqlc.arg('document'), sqlc.narg('created_by'), sqlc.arg('at'));

-- name: ListConfigTemplates :many
-- Every version of every template, newest version first within a key.
SELECT id, tenant_id, key, version, name, origin, document, created_by, created_at
FROM config_template
ORDER BY key, version DESC;

-- name: FindConfigTemplate :one
SELECT id, tenant_id, key, version, name, origin, document, created_by, created_at
FROM config_template WHERE key = sqlc.arg('key') AND version = sqlc.arg('version');

-- name: UpsertConfigTemplateUse :exec
INSERT INTO config_template_use (tenant_id, template_key, version, document, applied_at)
VALUES (current_tenant_id(), sqlc.arg('template_key'), sqlc.arg('version'), sqlc.arg('document'), sqlc.arg('at'))
ON CONFLICT (tenant_id, template_key)
DO UPDATE SET version = EXCLUDED.version, document = EXCLUDED.document, applied_at = EXCLUDED.applied_at;

-- name: ListConfigTemplateUses :many
SELECT tenant_id, template_key, version, document, applied_at
FROM config_template_use
ORDER BY template_key;
