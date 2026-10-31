-- A collection's public form: a request raised without signing in (UC-SVC-13).
--
-- One link per collection: a new one replaces the row and the old token stops answering. The
-- token is never stored - its keyed hash is, unique across the installation, so the
-- unauthenticated route finds its row by one index probe inside the tenant the token names (the
-- page link's way, migration 0262). A submission is filed as the person who made the link, so it
-- goes with them.
--
-- Expand only (rule 12): a new table nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS public_form (
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  collection_id uuid NOT NULL,
  token_hash    bytea NOT NULL,
  created_by    uuid NOT NULL,
  created_at    timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, collection_id),
  CONSTRAINT public_form_collection_fkey FOREIGN KEY (tenant_id, collection_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT public_form_created_by_fkey FOREIGN KEY (tenant_id, created_by)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS public_form_token_uq ON public_form (token_hash);

ALTER TABLE public_form ENABLE ROW LEVEL SECURITY;
ALTER TABLE public_form FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON public_form;
CREATE POLICY tenant_isolation ON public_form
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON public_form TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
