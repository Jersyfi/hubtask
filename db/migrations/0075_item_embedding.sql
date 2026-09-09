-- Semantic search's store, where the database can carry one (J-09, ADR-0050).
--
-- `0001_init` line 43 has carried `-- CREATE EXTENSION IF NOT EXISTS vector;` since the first day,
-- and this migration is what finally acts on it - by asking rather than demanding. Neither
-- `postgres:16-alpine` nor `ghcr.io/cloudnative-pg/postgresql:17.6` ships the extension, so an
-- unconditional CREATE EXTENSION would fail on every installation that upgrades without changing
-- its database image, and a failed migration is a stack that will not start.
--
-- So it asks `pg_available_extensions`, exactly as migration 0019 asks `pg_ts_config` before it
-- names a text search configuration. Where the extension is there, it is installed and the store
-- exists; where it is not, this migration does nothing at all and search stays lexical, complete,
-- and honest about itself in `/meta/capabilities`.
--
-- The embeddings live in their own table rather than in a column of `work_item`. A conditional
-- column would give one table two shapes and every query over it two meanings; a conditional table
-- is one object that is either there or not, which a capability check answers with one question -
-- and it keeps a vector out of the row that every write of an entry touches.

-- Forward-only and safe for a rolling update: one new table, conditionally, and nothing altered.

-- +goose Up

-- +goose StatementBegin
DO $vector$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'vector') THEN
    RAISE NOTICE 'pgvector is not available here; semantic search stays off (ADR-0050)';
    RETURN;
  END IF;

  CREATE EXTENSION IF NOT EXISTS vector;

  -- 1536 dimensions: what the OpenAI-compatible embedding models in common use produce, and what
  -- an installation configuring a model of another width has to re-index for. The width is fixed
  -- in the column because an index is built for one geometry - a table holding two would rank the
  -- mixture by nothing, which is the failure core/port/ai.EmbeddingResult carries its model to
  -- prevent.
  CREATE TABLE IF NOT EXISTS item_embedding (
    tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
    item_id    uuid NOT NULL,
    -- The model that produced the vector. Vectors are only comparable within one model's space, so
    -- a row that does not name its model is one nothing can decide about after a reconfiguration.
    model      text NOT NULL CHECK (length(model) BETWEEN 1 AND 200),
    embedding  vector(1536) NOT NULL,
    -- What the vector was made from, so a re-embedding pass can tell an entry whose text moved
    -- from one whose text did not - the digest core/domain/model/suggestion.Digest produces.
    source_digest bytea NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, item_id),
    -- Composite, like every tenant-scoped foreign key in this schema (ADR-0024): an embedding
    -- belongs to an item *of this workspace*, and the pair is what says so.
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
  );

  ALTER TABLE item_embedding ENABLE ROW LEVEL SECURITY;
  ALTER TABLE item_embedding FORCE ROW LEVEL SECURITY;
  -- CREATE POLICY has no IF NOT EXISTS, and this block may run on a database where an earlier
  -- attempt got this far, so the policy is dropped first.
  DROP POLICY IF EXISTS tenant_isolation ON item_embedding;
  CREATE POLICY tenant_isolation ON item_embedding
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

  GRANT SELECT, INSERT, UPDATE, DELETE ON item_embedding TO hubtask_app;
END $vector$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
