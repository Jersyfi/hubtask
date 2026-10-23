-- Saved presets and the first start's question (UC-WRK-30, domain-model.md §2).
--
-- `structure_preset` holds a preset saved from a hub's or collection's structure: the workspace's
-- (`WORKSPACE`, offered to everyone in it) or one person's own (`OWN`, offered to them alone, gone
-- with their account). `settings` is the eight settings in the document a layer's `settings` key
-- holds, its categories left out. The name is the person's content (rule 10). No change stamp: the
-- archive writes the table whole, as a withdrawn preset leaves no deletion marker a delta could carry.
--
-- `tenant.starter_answered_at` records that the workspace's first start was answered or skipped,
-- so nobody is asked twice.
--
-- Expand only: a new table nothing older reads, and a nullable column nothing older writes
-- (rule 12). The index is built without CONCURRENTLY: the table is created in this same migration.

-- +goose Up
CREATE TABLE IF NOT EXISTS structure_preset (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  id         uuid NOT NULL,
  origin     text NOT NULL,
  owner_id   uuid,
  name       text NOT NULL,
  settings   jsonb NOT NULL,
  created_at timestamptz NOT NULL,
  version    integer NOT NULL DEFAULT 1,
  PRIMARY KEY (tenant_id, id),
  CONSTRAINT structure_preset_origin_check CHECK (origin IN ('WORKSPACE', 'OWN')),
  CONSTRAINT structure_preset_owner_check CHECK ((origin = 'OWN') = (owner_id IS NOT NULL)),
  CONSTRAINT structure_preset_name_check CHECK (char_length(name) BETWEEN 1 AND 100),
  CONSTRAINT structure_preset_owner_fkey FOREIGN KEY (tenant_id, owner_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
-- What a person is offered: the workspace's, and their own.
CREATE INDEX IF NOT EXISTS structure_preset_origin_owner_idx
  ON structure_preset (tenant_id, origin, owner_id);

ALTER TABLE structure_preset ENABLE ROW LEVEL SECURITY;
ALTER TABLE structure_preset FORCE ROW LEVEL SECURITY;
-- +goose StatementBegin
DO $policy$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'structure_preset' AND policyname = 'tenant_isolation') THEN
    CREATE POLICY tenant_isolation ON structure_preset
      USING (tenant_id = current_tenant_id())
      WITH CHECK (tenant_id = current_tenant_id());
  END IF;
END $policy$;
-- +goose StatementEnd

GRANT SELECT, INSERT, UPDATE, DELETE ON structure_preset TO hubtask_app;

ALTER TABLE tenant ADD COLUMN IF NOT EXISTS starter_answered_at timestamptz;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
