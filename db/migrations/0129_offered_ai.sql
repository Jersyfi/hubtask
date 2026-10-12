-- AI offered by the installation (ADR-0072).
--
-- **Offered models are `ai_provider` rows with no workspace**, the shape migration 0103 gave
-- `identity_provider`: the key moves from the workspace to an `id`, `tenant_id` becomes nullable,
-- and the one policy becomes three - a read that also admits the installation's rows, because the
-- adapter of every workspace that chose one needs the row and its key to make the call; a write
-- that does not; and the installation's own scope, where `current_tenant_id()` is NULL, as the only
-- way to write them. A workspace is never *answered* more of an offered row than its name, its
-- processor and its jurisdiction - that bound is the application's, as it is for the installation's
-- sign-in providers.
--
-- **An offered row never carries consent.** Switching an offered model on is each workspace's own
-- act (ADR-0072 §3), stored on the workspace's side, so the row's `processing_allowed` is false by a
-- CHECK rather than by convention.
--
-- **The workspace's choice is `ai_source`**: which source answers it and, for an offered one,
-- which model. It carries consent, so it is the workspace's alone under the ordinary policy, and a
-- withdrawn offered model sets the reference to NULL rather than deleting the choice: the
-- workspace then gets no answer until it chooses again, which is the "stops, never deletes" of
-- ADR-0072 §5.
--
-- **Vectors of an offered model are stored under a qualified name** (`offered:<id>/<model>`), so
-- that a workspace's own model and an offered one with the same name are two spaces. The bound on
-- `item_embedding.model` widens to hold it.
--
-- **The rolling window**, named rather than implied. The previous binary's `ON CONFLICT (tenant_id)`
-- keeps an index to infer: the unique index on `tenant_id` below (NULLs are distinct, so it admits
-- any number of offered rows). Its reads have no `WHERE` and rely on the policy: once an operator
-- has offered a model, an old pod may read the offered row instead of the workspace's own, and that
-- workspace's own AI does not answer on that pod for the minutes of the rollout. It fails closed -
-- the offered row's consent is false by the CHECK, so nothing is sent anywhere.

-- +goose Up

ALTER TABLE ai_provider
  ADD COLUMN IF NOT EXISTS id           uuid DEFAULT gen_random_uuid(),
  -- What a workspace is shown of an offered model: its name and who processes what is sent to it,
  -- both the operator's declaration. Empty on a workspace's own row.
  ADD COLUMN IF NOT EXISTS display_name text NOT NULL DEFAULT '' CHECK (length(display_name) <= 200),
  ADD COLUMN IF NOT EXISTS processor    text NOT NULL DEFAULT '' CHECK (length(processor) <= 200);

UPDATE ai_provider SET id = gen_random_uuid() WHERE id IS NULL;

ALTER TABLE ai_provider ALTER COLUMN id SET NOT NULL;
ALTER TABLE ai_provider DROP CONSTRAINT IF EXISTS ai_provider_pkey;
ALTER TABLE ai_provider ADD CONSTRAINT ai_provider_pkey PRIMARY KEY (id);
ALTER TABLE ai_provider ALTER COLUMN tenant_id DROP NOT NULL;

-- One own row per workspace, as the primary key said until now. A plain unique index rather than
-- a partial one, so that the previous binary's `ON CONFLICT (tenant_id)` still infers it.
CREATE UNIQUE INDEX IF NOT EXISTS ai_provider_tenant_uq ON ai_provider (tenant_id);

-- Workspaces pick an offered model by name, so two offered rows never share one.
CREATE UNIQUE INDEX IF NOT EXISTS ai_provider_offered_name_uq
  ON ai_provider (display_name) WHERE tenant_id IS NULL;

ALTER TABLE ai_provider ADD CONSTRAINT ai_provider_offered_shape CHECK (
  tenant_id IS NOT NULL OR (
    display_name <> '' AND processor <> '' AND kind <> 'NOOP' AND NOT processing_allowed
  )
);

DROP POLICY IF EXISTS tenant_isolation ON ai_provider;

CREATE POLICY tenant_read ON ai_provider FOR SELECT
  USING (tenant_id = current_tenant_id() OR tenant_id IS NULL);

CREATE POLICY tenant_write ON ai_provider FOR ALL
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

CREATE POLICY installation_write ON ai_provider FOR ALL
  USING (tenant_id IS NULL AND current_tenant_id() IS NULL)
  WITH CHECK (tenant_id IS NULL AND current_tenant_id() IS NULL);

CREATE TABLE IF NOT EXISTS ai_source (
  tenant_id           uuid PRIMARY KEY REFERENCES tenant(id) ON DELETE CASCADE,
  source              text NOT NULL CHECK (source IN ('OWN', 'OFFERED', 'NONE')),
  -- The offered model chosen. NULL for any other source, and NULL once the operator withdrew the
  -- model chosen: the workspace then gets no answer until it chooses again.
  offered_provider_id uuid REFERENCES ai_provider(id) ON DELETE SET NULL,
  updated_at          timestamptz NOT NULL,
  version             integer NOT NULL DEFAULT 1,
  CONSTRAINT ai_source_offered_only CHECK (source = 'OFFERED' OR offered_provider_id IS NULL)
);

ALTER TABLE ai_source ENABLE ROW LEVEL SECURITY;
ALTER TABLE ai_source FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ai_source
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

GRANT SELECT, INSERT, UPDATE, DELETE ON ai_source TO hubtask_app;

-- The embeddings exist only where pgvector does (migration 0075), so the wider bound is applied
-- only where the table is. NOT VALID: every stored row already meets a wider bound, and checking
-- them again would hold a lock over the table for nothing.
-- +goose StatementBegin
DO $widen$
BEGIN
  IF to_regclass('item_embedding') IS NOT NULL THEN
    ALTER TABLE item_embedding DROP CONSTRAINT IF EXISTS item_embedding_model_check;
    ALTER TABLE item_embedding ADD CONSTRAINT item_embedding_model_check
      CHECK (length(model) BETWEEN 1 AND 300) NOT VALID;
  END IF;
END $widen$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
