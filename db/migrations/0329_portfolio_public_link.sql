-- A portfolio's roadmap opened read-only by a link, without signing in (UC-GOL-03 check 15; the
-- owner's fourth answers, item 9).
--
-- A page's public link again (migration 0269): one link per portfolio, a new one replacing the row
-- so the old token stops answering; only the token's keyed hash is kept, unique across the
-- installation, so the unauthenticated route finds its row by one index probe inside the tenant
-- the token names. The link reads as the person who made it, so it goes with them.
--
-- Expand only (rule 12): a new table nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS portfolio_public_link (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  portfolio_id uuid NOT NULL,
  token_hash   bytea NOT NULL,
  created_by   uuid NOT NULL,
  created_at   timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, portfolio_id),
  CONSTRAINT portfolio_public_link_portfolio_fkey FOREIGN KEY (tenant_id, portfolio_id)
    REFERENCES portfolio (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT portfolio_public_link_created_by_fkey FOREIGN KEY (tenant_id, created_by)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS portfolio_public_link_token_uq ON portfolio_public_link (token_hash);

ALTER TABLE portfolio_public_link ENABLE ROW LEVEL SECURITY;
ALTER TABLE portfolio_public_link FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON portfolio_public_link;
CREATE POLICY tenant_isolation ON portfolio_public_link
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON portfolio_public_link TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
