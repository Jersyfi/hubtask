-- The server's clock per field (N-05, offline-sync.md §4.2, §10).
--
-- "Last writer wins per field via the HLC" needs the server to know the reading its own copy of a
-- field carries, and nothing stored one: the change log has it, one entry per field, but reading
-- the latest entry of a field is a scan over a partitioned stream that retention empties. So a row
-- per field that has moved since this migration: which entity, which field, and the reading of
-- the write that landed - a server reading for a write over the API, the device's for a write a
-- push applied. A push compares its reading against this row and wins or loses per field.
--
-- Not backfilled. A field written before this migration has no row and loses to the first device
-- that writes it, whatever its clock says - last writer wins over a value nobody stamped, bounded
-- by the skew rule, and the honest answer for a reading the log kept only partially
-- (docs/backlog/milestone-0.8.5.md, the header).
--
-- The key is the entity's rather than a foreign key to one table, because the change log names
-- entities by kind and the rule applies to more than entries - a container's name merges the same
-- way. An entry's rows go when the entry is purged - the purge removes them beside the entry - and
-- the other kinds arrive with their own tasks.
--
-- Expand only: a new table, read and written by the new version alone.

-- +goose Up

CREATE TABLE IF NOT EXISTS field_clock (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  entity     text NOT NULL,
  entity_id  uuid NOT NULL,
  field      text NOT NULL,
  hlc        text NOT NULL,                        -- physical:counter:device, sorts as a clock
  PRIMARY KEY (tenant_id, entity, entity_id, field)
);

ALTER TABLE field_clock ENABLE ROW LEVEL SECURITY;
ALTER TABLE field_clock FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON field_clock
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

GRANT SELECT, INSERT, UPDATE, DELETE ON field_clock TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
