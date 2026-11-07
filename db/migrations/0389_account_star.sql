-- A person's stars (navigation concept §6.2, UC-WRK-37): one ordered list per account, written
-- whole, as products_seen is, and guarded by its version because two devices reorder it. Each
-- member is {kind, id}; the id names a row of the table its kind says, so it carries no foreign key
-- - a star whose object is gone or unreadable is not answered and the next write drops it.
--
-- Expand only (rule 12): a new table nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS account_star (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id uuid NOT NULL,
  stars      jsonb NOT NULL DEFAULT '[]'
             CHECK (jsonb_typeof(stars) = 'array' AND jsonb_array_length(stars) <= 100),
  version    integer NOT NULL CHECK (version >= 1),
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, account_id),
  FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE account_star ENABLE ROW LEVEL SECURITY;
ALTER TABLE account_star FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON account_star;
CREATE POLICY tenant_isolation ON account_star
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON account_star TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
