-- A member's confirmation that they read the notice of an emergency access (ADR-0073 §4, UC-ID-16
-- check 10).
--
-- One row per member per access, written once: a second confirmation finds the row and writes
-- nothing. Identifiers and a moment, never the reason. The row goes with its access, and so with the
-- hub, and with the member's account. Only the member reads it; the owner who opened the hub is not
-- shown who confirmed (the workspace's trail says it).
--
-- In the archive and the workspace export, with its access: it is the proof that the member was
-- told, and a restore that brought the access back without it would ask the member again. In the
-- person's own export as theirs (`account_id`).
--
-- Expand only (rule 12): a new table that a pod of the previous release neither reads nor writes.
-- Built plainly, not CONCURRENTLY, because nothing reads it yet.

-- +goose Up
CREATE TABLE IF NOT EXISTS private_hub_access_acknowledgement (
  tenant_id       uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  access_id       uuid NOT NULL,
  account_id      uuid NOT NULL,
  acknowledged_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, access_id, account_id),
  CONSTRAINT private_hub_access_acknowledgement_access_id_fkey
    FOREIGN KEY (tenant_id, access_id) REFERENCES private_hub_access (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT private_hub_access_acknowledgement_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
-- The account's cascade finds its rows without reading the whole table; the access's uses the key.
CREATE INDEX IF NOT EXISTS private_hub_access_acknowledgement_account_idx
  ON private_hub_access_acknowledgement (tenant_id, account_id);

ALTER TABLE private_hub_access_acknowledgement ENABLE ROW LEVEL SECURITY;
ALTER TABLE private_hub_access_acknowledgement FORCE ROW LEVEL SECURITY;
-- +goose StatementBegin
DO $policy$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'private_hub_access_acknowledgement' AND policyname = 'tenant_isolation') THEN
    CREATE POLICY tenant_isolation ON private_hub_access_acknowledgement
      USING (tenant_id = current_tenant_id())
      WITH CHECK (tenant_id = current_tenant_id());
  END IF;
END $policy$;
-- +goose StatementEnd

GRANT SELECT, INSERT, UPDATE, DELETE ON private_hub_access_acknowledgement TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
