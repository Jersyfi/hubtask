-- The workspace owner's emergency access to a private hub (ADR-0073 §4, UC-ID-16 check 4).
--
-- **The access.** One row per opening: the hub, who opened it, the reason they gave, and the hour.
-- The reason is written to the hub's members and kept as their record of who opened their hub, as
-- long as the hub exists; the row goes with the hub. An opener who is deleted leaves the row with
-- nobody named, as every attributed act does.
--
-- **The notice.** Every member is told in the transaction that opens the hub, by a notification of
-- the new category `EMERGENCY_ACCESS` that names the access as its subject, so that the mail reads
-- the opener, the time and the reason at delivery and the record holds no content.
--
-- Expand only (rule 12). A pod of the previous release reads no column added here, inserts no
-- category it does not know, and never delivers an `EMERGENCY_ACCESS` record: those travel under a
-- job kind of their own that it has no handler for. The two category checks are widened in place, as
-- migration 0062 widened them; every stored row already satisfies the wider sets.

-- +goose Up
CREATE TABLE IF NOT EXISTS private_hub_access (
  id        uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id    uuid NOT NULL,
  opened_by uuid,
  reason    text NOT NULL CHECK (char_length(reason) BETWEEN 1 AND 500),
  opened_at timestamptz NOT NULL,
  ends_at   timestamptz NOT NULL,
  CONSTRAINT private_hub_access_hour_check CHECK (ends_at > opened_at),
  UNIQUE (tenant_id, id),
  CONSTRAINT private_hub_access_hub_id_fkey
    FOREIGN KEY (tenant_id, hub_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT private_hub_access_opened_by_fkey
    FOREIGN KEY (tenant_id, opened_by) REFERENCES account (tenant_id, id) ON DELETE SET NULL (opened_by)
);
-- A new table nobody reads yet: built plainly, not CONCURRENTLY.
CREATE INDEX IF NOT EXISTS private_hub_access_hub_idx
  ON private_hub_access (tenant_id, hub_id, opened_at DESC);

ALTER TABLE private_hub_access ENABLE ROW LEVEL SECURITY;
ALTER TABLE private_hub_access FORCE ROW LEVEL SECURITY;
-- +goose StatementBegin
DO $policy$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'private_hub_access' AND policyname = 'tenant_isolation') THEN
    CREATE POLICY tenant_isolation ON private_hub_access
      USING (tenant_id = current_tenant_id())
      WITH CHECK (tenant_id = current_tenant_id());
  END IF;
END $policy$;
-- +goose StatementEnd

GRANT SELECT, INSERT, UPDATE, DELETE ON private_hub_access TO hubtask_app;

ALTER TABLE notification ADD COLUMN IF NOT EXISTS private_hub_access_id uuid;
ALTER TABLE notification ADD CONSTRAINT notification_private_hub_access_id_fkey
  FOREIGN KEY (tenant_id, private_hub_access_id) REFERENCES private_hub_access (tenant_id, id)
  ON DELETE CASCADE NOT VALID;
ALTER TABLE notification VALIDATE CONSTRAINT notification_private_hub_access_id_fkey;

ALTER TABLE notification DROP CONSTRAINT notification_one_subject_check;
ALTER TABLE notification ADD CONSTRAINT notification_one_subject_check
  CHECK (num_nonnulls(item_id, rule_id, subscription_id, private_hub_access_id) <= 1);

ALTER TABLE notification DROP CONSTRAINT notification_category_check;
ALTER TABLE notification ADD CONSTRAINT notification_category_check
  CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER','INTEGRATION','RETENTION','EMERGENCY_ACCESS'));

-- The preference table carries the same closed set. Nobody can store a preference for the new
-- category - it is never switched off - but the set stays one set in both places.
ALTER TABLE notification_preference DROP CONSTRAINT notification_preference_category_check;
ALTER TABLE notification_preference ADD CONSTRAINT notification_preference_category_check
  CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER','INTEGRATION','RETENTION','EMERGENCY_ACCESS'));

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
