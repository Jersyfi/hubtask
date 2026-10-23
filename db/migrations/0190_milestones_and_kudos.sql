-- Milestones and kudos (UC-WRK-29 checks 4 to 6, design-system.md §7).
--
-- `container_milestone` holds the last moment a collection, or every collection of a hub, was left
-- with nothing open: one row per container, moved by a later finish, gone with the container. It
-- names nobody - a milestone is the collection's, not a person's - and is archived with it.
--
-- `kudos` holds one person's acknowledgement of another's completion of an entry: who gave it, to
-- whom, when. Personal data about both (data-catalog.md): read only by the person it was given to,
-- never audited or announced, and in no workspace's archive - only in the copy each of the two gets
-- of their own data; gone with the entry, with either account, and with an erasure of either person.
--
-- Expand only: two new tables nothing older reads or writes (rule 12). Their indexes are built
-- without CONCURRENTLY: the tables are created in this same migration and nobody reads them yet.

-- +goose Up
CREATE TABLE IF NOT EXISTS container_milestone (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  container_id uuid NOT NULL,
  reached_at   timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, container_id),
  CONSTRAINT container_milestone_container_fkey FOREIGN KEY (tenant_id, container_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS kudos (
  tenant_id uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id   uuid NOT NULL,
  given_by  uuid NOT NULL,
  given_to  uuid NOT NULL,
  given_at  timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, item_id, given_by, given_to),
  CONSTRAINT kudos_not_to_oneself CHECK (given_by <> given_to),
  CONSTRAINT kudos_item_fkey FOREIGN KEY (tenant_id, item_id)
    REFERENCES work_item (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT kudos_given_by_fkey FOREIGN KEY (tenant_id, given_by)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT kudos_given_to_fkey FOREIGN KEY (tenant_id, given_to)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
-- What the person they were given to reads on an entry, and what an erasure removes by either side.
CREATE INDEX IF NOT EXISTS kudos_given_to_idx ON kudos (tenant_id, given_to, item_id);
CREATE INDEX IF NOT EXISTS kudos_given_by_idx ON kudos (tenant_id, given_by);

ALTER TABLE container_milestone ENABLE ROW LEVEL SECURITY;
ALTER TABLE container_milestone FORCE ROW LEVEL SECURITY;
ALTER TABLE kudos ENABLE ROW LEVEL SECURITY;
ALTER TABLE kudos FORCE ROW LEVEL SECURITY;
-- +goose StatementBegin
DO $policy$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'container_milestone' AND policyname = 'tenant_isolation') THEN
    CREATE POLICY tenant_isolation ON container_milestone
      USING (tenant_id = current_tenant_id())
      WITH CHECK (tenant_id = current_tenant_id());
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'kudos' AND policyname = 'tenant_isolation') THEN
    CREATE POLICY tenant_isolation ON kudos
      USING (tenant_id = current_tenant_id())
      WITH CHECK (tenant_id = current_tenant_id());
  END IF;
END $policy$;
-- +goose StatementEnd

GRANT SELECT, INSERT, UPDATE, DELETE ON container_milestone TO hubtask_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON kudos TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
