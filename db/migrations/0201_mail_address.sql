-- Mail addresses for every place, on the operator's receiving domain (ADR-0092).
--
-- One row per address. The code is the key: sixteen random base32 characters, unique across the
-- installation, because the inbound route has no tenant until the code names one. Kept readable
-- rather than hashed, since a member copies the address again whenever they need it; what bounds
-- it is row level security and the place's own read permission.
--
-- At most one live address per place: replacing one revokes the old row in the same transaction,
-- and the partial unique index refuses a second live one. No foreign key to the place - an entry
-- or a collection that is purged takes nothing with it here, and a delivery to its address finds
-- no place and answers not-found.
--
-- `opened_by` is who opened the address to anyone (ADR-0092 decision 5): an unknown sender's mail
-- is written as them. `window_start` and `window_count` are the per-address hour the route counts
-- deliveries in (decision 7).
--
-- `jumble_entry` gains the place a mail was meant for, where it landed in the jumble instead.
-- Expand only: nullable columns without a default.

-- +goose Up
CREATE TABLE IF NOT EXISTS mail_address (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  target_kind   text NOT NULL CHECK (target_kind IN ('JUMBLE','COLLECTION','ITEM')),
  target_id     uuid,
  code          text NOT NULL,
  sender_policy text NOT NULL DEFAULT 'MEMBERS' CHECK (sender_policy IN ('MEMBERS','ANYONE')),
  opened_by     uuid,
  created_by    uuid,
  created_at    timestamptz NOT NULL,
  revoked_at    timestamptz,
  window_start  timestamptz,
  window_count  integer NOT NULL DEFAULT 0,
  CHECK ((target_kind = 'JUMBLE') = (target_id IS NULL)),
  UNIQUE (tenant_id, id)
);

CREATE UNIQUE INDEX IF NOT EXISTS mail_address_code_uq ON mail_address (code);
CREATE UNIQUE INDEX IF NOT EXISTS mail_address_live_uq ON mail_address
  (tenant_id, target_kind, coalesce(target_id, '00000000-0000-0000-0000-000000000000'::uuid))
  WHERE revoked_at IS NULL;

ALTER TABLE mail_address ENABLE ROW LEVEL SECURITY;
ALTER TABLE mail_address FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON mail_address
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

GRANT SELECT, INSERT, UPDATE ON mail_address TO hubtask_app;

-- +goose StatementBegin
-- The door reads across tenants, so its owner holds the standing read policy the other doors'
-- owners hold (multi-tenancy.md §2.1).
DO $owner$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'mail_address' AND policyname = 'doors_read') THEN
    CREATE POLICY doors_read ON mail_address FOR SELECT TO CURRENT_USER USING (true);
  END IF;
END $owner$;
-- +goose StatementEnd

-- +goose StatementBegin
-- The inbound route's one question before it has a tenant: whose live address is this code?
-- Narrow by construction, `resolve_tenant`'s shape: one identifier or none, never a listing, and
-- nothing about the place - what is done with the tenant stays behind row level security.
CREATE OR REPLACE FUNCTION resolve_mail_address(address_code text) RETURNS uuid
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT tenant_id FROM mail_address
  WHERE code = lower(address_code) AND revoked_at IS NULL
  LIMIT 1
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION resolve_mail_address(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION resolve_mail_address(text) TO hubtask_app;

ALTER TABLE jumble_entry ADD COLUMN IF NOT EXISTS meant_for_kind text;
ALTER TABLE jumble_entry ADD COLUMN IF NOT EXISTS meant_for_id uuid;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
