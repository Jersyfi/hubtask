-- A page opened read-only by a link, without signing in (M3; UC-DOC-08 check 5).
--
-- One link per page: a new one replaces the row and the old token stops answering. The token is
-- never stored - its keyed hash is, unique across the installation, so the unauthenticated route
-- finds its row by one index probe inside the tenant the token names (the calendar feed's way,
-- multi-tenancy.md §2.2). The link reads as the person who made it, so it goes with them.
--
-- Expand only (rule 12): a new table nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS page_public_link (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  page_id    uuid NOT NULL,
  token_hash bytea NOT NULL,
  created_by uuid NOT NULL,
  created_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, page_id),
  CONSTRAINT page_public_link_page_fkey FOREIGN KEY (tenant_id, page_id)
    REFERENCES page (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT page_public_link_created_by_fkey FOREIGN KEY (tenant_id, created_by)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS page_public_link_token_uq ON page_public_link (token_hash);

ALTER TABLE page_public_link ENABLE ROW LEVEL SECURITY;
ALTER TABLE page_public_link FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON page_public_link;
CREATE POLICY tenant_isolation ON page_public_link
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON page_public_link TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
