-- A requester without an account (accepted.md B, data-protection.md §4): an address that wrote in
-- by mail and whose request an account filed on its behalf, with its name, its first and last
-- contact and the entries it wrote on. REQUESTER removes it 24 months after the last contact; an
-- erasure case by address removes it at once.
--
-- Expand only (rule 12): two new tables nothing older reads or writes.

-- +goose Up
CREATE TABLE requester (
  id               uuid PRIMARY KEY,
  tenant_id        uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  address          text NOT NULL CHECK (char_length(address) BETWEEN 3 AND 320 AND address = lower(address)),
  display_name     text NOT NULL DEFAULT '' CHECK (char_length(display_name) <= 200),
  first_contact_at timestamptz NOT NULL,
  last_contact_at  timestamptz NOT NULL,
  CHECK (last_contact_at >= first_contact_at),
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, address)
);
CREATE INDEX requester_last_contact_idx ON requester (tenant_id, last_contact_at);
ALTER TABLE requester ENABLE ROW LEVEL SECURITY;
ALTER TABLE requester FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON requester
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON requester TO hubtask_app;

CREATE TABLE requester_contact (
  tenant_id    uuid NOT NULL,
  requester_id uuid NOT NULL,
  item_id      uuid NOT NULL,
  at           timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, requester_id, item_id),
  CONSTRAINT requester_contact_requester_fkey FOREIGN KEY (tenant_id, requester_id)
    REFERENCES requester (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT requester_contact_item_fkey FOREIGN KEY (tenant_id, item_id)
    REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX requester_contact_item_idx ON requester_contact (tenant_id, item_id);
ALTER TABLE requester_contact ENABLE ROW LEVEL SECURITY;
ALTER TABLE requester_contact FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON requester_contact
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON requester_contact TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
