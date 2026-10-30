-- An entry's status history (UC-GOL-04 check 11): each time an entry enters a status, the status
-- and the moment, so a summary measures cycle time, WIP age and time in status. Expand only (rule
-- 12): a new table and a trigger a pod of the previous release writes through like any other.
--
-- Nothing recorded these moments before; the history counts from this migration on, as the flow
-- moments of 0194 do: an entry without a row holds none of the three measures. The key is
-- recorded, not its category - a workflow's shape is a document a statement does not look into,
-- and the use case hands the compiler the keys of the IN_PROGRESS category instead.
--
-- Written by a trigger rather than by each use case that sets a status, for 0194's reason: a
-- status is set by creating, a step, completion, a move to another collection, a change of type
-- and a device's push, and one that forgot it would drop entries from the figure. The moment is the
-- row's own `created_at` or `updated_at`, which the application sets from its clock (rule 4).
-- Derived and not archived (tenant-export.md §6): a restored entry's history starts at its restore.
-- +goose Up
CREATE TABLE IF NOT EXISTS entry_status_change (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id    uuid NOT NULL,
  seq        bigint GENERATED ALWAYS AS IDENTITY,
  status     text NOT NULL,
  entered_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, item_id, seq),
  CONSTRAINT entry_status_change_item_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
ALTER TABLE entry_status_change ENABLE ROW LEVEL SECURITY;
ALTER TABLE entry_status_change FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON entry_status_change;
CREATE POLICY tenant_isolation ON entry_status_change
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON entry_status_change TO hubtask_app;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION work_item_status_history() RETURNS trigger LANGUAGE plpgsql AS
$$
BEGIN
  IF NEW.status IS NULL THEN
    RETURN NULL;
  END IF;
  IF TG_OP = 'UPDATE' THEN
    IF NEW.status IS NOT DISTINCT FROM OLD.status THEN
      RETURN NULL;
    END IF;
    INSERT INTO entry_status_change (tenant_id, item_id, status, entered_at)
    VALUES (NEW.tenant_id, NEW.id, NEW.status, NEW.updated_at);
  ELSE
    INSERT INTO entry_status_change (tenant_id, item_id, status, entered_at)
    VALUES (NEW.tenant_id, NEW.id, NEW.status, NEW.created_at);
  END IF;
  RETURN NULL;
END
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS work_item_status_history ON work_item;
CREATE TRIGGER work_item_status_history
  AFTER INSERT OR UPDATE OF status ON work_item
  FOR EACH ROW EXECUTE FUNCTION work_item_status_history();

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
