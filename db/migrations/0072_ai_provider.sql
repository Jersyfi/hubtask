-- The workspace's AI provider (J-02, ADR-0012, ADR-0049): which provider it would ask, under
-- which models, in which jurisdiction, and whether it consents to being asked at all.
--
-- What is stored is never a usable credential. The API key is sealed under E-02's envelope and
-- opened only by the adapter that makes the call, exactly as `identity_provider.client_secret_enc`
-- is opened only by the token exchange.
--
-- One row per workspace, and the primary key says so: a second configuration cannot exist, so no
-- code path has to decide which of two providers wins.

-- Forward-only and safe for a rolling update: one new table, nothing altered.

-- +goose Up

CREATE TABLE IF NOT EXISTS ai_provider (
  tenant_id        uuid PRIMARY KEY REFERENCES tenant(id) ON DELETE CASCADE,
  -- Which adapter answers. A closed set, checked here as well as in the domain, because a value
  -- outside it would reach the composition root as a provider nobody can build.
  kind             text NOT NULL CHECK (kind IN ('NOOP', 'OPENAI_COMPATIBLE', 'OLLAMA')),
  -- The endpoint the adapter calls, empty for NOOP. It is configuration rather than data, and
  -- every call to it goes through the guarded client, which is what stops a typed address
  -- reaching this installation's own network (ADR-0015, T-07).
  base_url         text NOT NULL DEFAULT '' CHECK (length(base_url) <= 2000),
  completion_model text NOT NULL DEFAULT '' CHECK (length(completion_model) <= 200),
  embedding_model  text NOT NULL DEFAULT '' CHECK (length(embedding_model) <= 200),
  -- Sealed, never hashed: the adapter needs the plaintext to sign a request, so this is the E-02
  -- envelope and its key label rather than a digest. Nullable, because a local model usually
  -- needs no key at all and an empty envelope would be a lie about what is stored.
  api_key_enc      bytea,
  api_key_key_id   text,
  -- Where the provider processes what is sent to it, as the operator declares it. A declaration
  -- rather than something this software can verify - its purpose is that the decision is
  -- documented rather than made by accident (ADR-0018 decision 7, data-protection.md §6).
  jurisdiction     text NOT NULL
    CHECK (jurisdiction IN ('SELF_HOSTED', 'EEA', 'ADEQUACY', 'THIRD_COUNTRY')),
  -- ai-first.md §2's switch, checked before every call. False by default and on every row that
  -- does not say otherwise: consent is given, never inherited.
  processing_allowed boolean NOT NULL DEFAULT false,
  created_at       timestamptz NOT NULL,
  updated_at       timestamptz,
  version          integer NOT NULL DEFAULT 1,
  -- An envelope is two columns or neither. A ciphertext without its key label is unopenable and
  -- a label without a ciphertext is a key nobody is using, and both are states the resealer
  -- would have to invent a meaning for.
  CONSTRAINT ai_provider_key_envelope CHECK (
    (api_key_enc IS NULL AND api_key_key_id IS NULL) OR
    (api_key_enc IS NOT NULL AND api_key_key_id IS NOT NULL)
  )
);

-- Every tenant-scoped table is behind row level security, and one holding a sealed key is not
-- where the exception starts.
ALTER TABLE ai_provider ENABLE ROW LEVEL SECURITY;
ALTER TABLE ai_provider FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ai_provider
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

-- Explicit rather than left to the default privileges, because those follow the role that
-- creates the table and a migration is not always applied by the same one.
GRANT SELECT, INSERT, UPDATE, DELETE ON ai_provider TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
