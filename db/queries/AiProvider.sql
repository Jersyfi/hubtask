-- The workspace's AI provider (J-02).
--
-- Every statement here runs inside the transaction wrapper that sets `app.tenant_id`, so the
-- policy underneath answers "which workspace" - `current_tenant_id()` is written on insert rather
-- than passed in, the way every tenant-scoped insert in this schema is.

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
FROM ai_provider;

-- name: DeleteAiProvider :execrows
DELETE FROM ai_provider;

-- name: RewrapAiProviderKey :execrows
-- A re-seal (ADR-0045): the wrapping moves, the configuration does not, so the version stays -
-- an operator rotating the installation's keys has not changed anybody's provider.
UPDATE ai_provider
SET api_key_enc = sqlc.arg('api_key_enc'), api_key_key_id = sqlc.arg('api_key_key_id')
WHERE api_key_key_id = sqlc.arg('expected_key_id');
