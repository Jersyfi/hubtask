-- The workspace's AI provider, and the models the installation offers (ADR-0072).
--
-- Every statement here runs inside the transaction wrapper that sets `app.tenant_id`, so the
-- policy underneath answers "which workspace" - `current_tenant_id()` is written on insert rather
-- than passed in, the way every tenant-scoped insert in this schema is.
--
-- The read policy also admits the installation's offered rows (migration 0129), so every statement
-- about the workspace's own row names it: `tenant_id = current_tenant_id()`. Without that, a read
-- answering one row could answer an offered one, and a delete could reach for it.

-- name: UpsertAiProvider :one
-- Set whole, not patched: a provider half-changed is one nobody can reason about, and the
-- endpoint, the models and the key have to agree with each other. The version rises on every
-- write, so a concurrent second configuration is visible as a conflict.
INSERT INTO ai_provider
  (tenant_id, kind, base_url, completion_model, embedding_model,
   api_key_enc, api_key_key_id, jurisdiction, processing_allowed, created_at)
VALUES (
  current_tenant_id(), sqlc.arg('kind'), sqlc.arg('base_url'), sqlc.arg('completion_model'),
  sqlc.arg('embedding_model'), sqlc.narg('api_key_enc'), sqlc.narg('api_key_key_id'),
  sqlc.arg('jurisdiction'), sqlc.arg('processing_allowed'), sqlc.arg('now')
)
ON CONFLICT (tenant_id) DO UPDATE SET
  kind               = excluded.kind,
  base_url           = excluded.base_url,
  completion_model   = excluded.completion_model,
  embedding_model    = excluded.embedding_model,
  api_key_enc        = excluded.api_key_enc,
  api_key_key_id     = excluded.api_key_key_id,
  jurisdiction       = excluded.jurisdiction,
  processing_allowed = excluded.processing_allowed,
  updated_at         = sqlc.arg('now'),
  version            = ai_provider.version + 1
RETURNING kind, base_url, completion_model, embedding_model,
          (api_key_enc IS NOT NULL)::boolean AS has_api_key,
          jurisdiction, processing_allowed, created_at, updated_at, version;

-- name: UpsertAiProviderKeepingKey :one
-- The same write with the stored envelope left where it is, for a caller who changed a model and
-- sent no key. Its own statement rather than a branch inside the one above, because "keep what is
-- there" and "store this" are different intentions and a NULL cannot carry both.
INSERT INTO ai_provider
  (tenant_id, kind, base_url, completion_model, embedding_model,
   jurisdiction, processing_allowed, created_at)
VALUES (
  current_tenant_id(), sqlc.arg('kind'), sqlc.arg('base_url'), sqlc.arg('completion_model'),
  sqlc.arg('embedding_model'), sqlc.arg('jurisdiction'), sqlc.arg('processing_allowed'),
  sqlc.arg('now')
)
ON CONFLICT (tenant_id) DO UPDATE SET
  kind               = excluded.kind,
  base_url           = excluded.base_url,
  completion_model   = excluded.completion_model,
  embedding_model    = excluded.embedding_model,
  jurisdiction       = excluded.jurisdiction,
  processing_allowed = excluded.processing_allowed,
  updated_at         = sqlc.arg('now'),
  version            = ai_provider.version + 1
RETURNING kind, base_url, completion_model, embedding_model,
          (api_key_enc IS NOT NULL)::boolean AS has_api_key,
          jurisdiction, processing_allowed, created_at, updated_at, version;

-- name: FindAiProvider :one
-- What a reader is allowed to see: never the sealed key, only whether there is one. The single
-- caller that needs the plaintext asks for it by name below, so a read cannot spill it by
-- accident.
SELECT kind, base_url, completion_model, embedding_model,
       (api_key_enc IS NOT NULL)::boolean AS has_api_key,
       jurisdiction, processing_allowed, created_at, updated_at, version
FROM ai_provider;

-- name: FindAiProviderKey :one
-- The adapter's own read, separate from the one above so that opening the envelope is a
-- deliberate call and not a field that happens to be in a struct somebody logged.
SELECT kind, base_url, completion_model, embedding_model,
       api_key_enc, api_key_key_id, jurisdiction, processing_allowed
FROM ai_provider
WHERE tenant_id = current_tenant_id();

-- name: DeleteAiProvider :execrows
DELETE FROM ai_provider WHERE tenant_id = current_tenant_id();

-- name: RewrapAiProviderKey :execrows
-- A re-seal (ADR-0045): the wrapping moves, the configuration does not, so the version stays -
-- an operator rotating the installation's keys has not changed anybody's provider.
UPDATE ai_provider
SET api_key_enc = sqlc.arg('api_key_enc'), api_key_key_id = sqlc.arg('api_key_key_id')
WHERE tenant_id = current_tenant_id() AND api_key_key_id = sqlc.arg('expected_key_id');

-- name: ListOfferedAiProviders :many
-- The models the installation offers, never with the key. Read by the operator and, through the
-- read policy, by a workspace choosing one - which is answered only name, processor and
-- jurisdiction of it by the application.
SELECT id, display_name, processor, kind, base_url, completion_model, embedding_model,
       (api_key_enc IS NOT NULL)::boolean AS has_api_key,
       jurisdiction, created_at, updated_at, version
FROM ai_provider
WHERE tenant_id IS NULL
ORDER BY display_name COLLATE hubtask_name, id;

-- name: FindOfferedAiProvider :one
SELECT id, display_name, processor, kind, base_url, completion_model, embedding_model,
       (api_key_enc IS NOT NULL)::boolean AS has_api_key,
       jurisdiction, created_at, updated_at, version
FROM ai_provider
WHERE tenant_id IS NULL AND id = sqlc.arg('id');

-- name: FindOfferedAiProviderKey :one
-- The adapter's read of an offered model a workspace chose, inside that workspace's transaction:
-- the read policy admits the row, and the key is opened only by the caller that makes the call.
SELECT id, display_name, processor, kind, base_url, completion_model, embedding_model,
       api_key_enc, api_key_key_id, jurisdiction
FROM ai_provider
WHERE tenant_id IS NULL AND id = sqlc.arg('id');

-- name: InsertOfferedAiProvider :one
-- Written only in the installation's own scope, where `current_tenant_id()` is NULL and the
-- installation_write policy is the one that admits it.
INSERT INTO ai_provider
  (id, tenant_id, display_name, processor, kind, base_url, completion_model, embedding_model,
   api_key_enc, api_key_key_id, jurisdiction, processing_allowed, created_at)
VALUES (
  sqlc.arg('id'), NULL, sqlc.arg('display_name'), sqlc.arg('processor'), sqlc.arg('kind'),
  sqlc.arg('base_url'), sqlc.arg('completion_model'), sqlc.arg('embedding_model'),
  sqlc.narg('api_key_enc'), sqlc.narg('api_key_key_id'), sqlc.arg('jurisdiction'), false,
  sqlc.arg('now')
)
RETURNING id, display_name, processor, kind, base_url, completion_model, embedding_model,
          (api_key_enc IS NOT NULL)::boolean AS has_api_key,
          jurisdiction, created_at, updated_at, version;

-- name: UpdateOfferedAiProvider :one
-- Set whole, as the workspace's own row is, with the key replaced (NULL clears it).
UPDATE ai_provider SET
  display_name = sqlc.arg('display_name'), processor = sqlc.arg('processor'),
  kind = sqlc.arg('kind'), base_url = sqlc.arg('base_url'),
  completion_model = sqlc.arg('completion_model'), embedding_model = sqlc.arg('embedding_model'),
  api_key_enc = sqlc.narg('api_key_enc'), api_key_key_id = sqlc.narg('api_key_key_id'),
  jurisdiction = sqlc.arg('jurisdiction'),
  updated_at = sqlc.arg('now'), version = version + 1
WHERE tenant_id IS NULL AND id = sqlc.arg('id')
RETURNING id, display_name, processor, kind, base_url, completion_model, embedding_model,
          (api_key_enc IS NOT NULL)::boolean AS has_api_key,
          jurisdiction, created_at, updated_at, version;

-- name: UpdateOfferedAiProviderKeepingKey :one
UPDATE ai_provider SET
  display_name = sqlc.arg('display_name'), processor = sqlc.arg('processor'),
  kind = sqlc.arg('kind'), base_url = sqlc.arg('base_url'),
  completion_model = sqlc.arg('completion_model'), embedding_model = sqlc.arg('embedding_model'),
  jurisdiction = sqlc.arg('jurisdiction'),
  updated_at = sqlc.arg('now'), version = version + 1
WHERE tenant_id IS NULL AND id = sqlc.arg('id')
RETURNING id, display_name, processor, kind, base_url, completion_model, embedding_model,
          (api_key_enc IS NOT NULL)::boolean AS has_api_key,
          jurisdiction, created_at, updated_at, version;

-- name: DeleteOfferedAiProvider :execrows
-- Every workspace that chose it keeps its choice pointing at nothing (ON DELETE SET NULL), and
-- gets no answer until it chooses again.
DELETE FROM ai_provider WHERE tenant_id IS NULL AND id = sqlc.arg('id');
