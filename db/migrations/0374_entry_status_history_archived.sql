-- An entry's status history travels in the archive (UC-GOL-04 check 11, tenant-export.md §6). Until
-- now a restored entry's history started at its restore (migration 0350), so its cycle time, WIP age
-- and time in status were lost by every restore. The archive now carries the rows, and a restore
-- writes them back after the entries.
--
-- `from_restore` marks the row the trigger writes while a restore writes an entry: a stand-in for a
-- history the archive may or may not carry. The restore's write of the entry's first archived row
-- removes it; an entry whose archive carries no history - an archive from before this release, an
-- entry from before migration 0250 - keeps it, and its history starts at the restore as before.
--
-- Expand only (rule 12): one column with a constant default, and the trigger function replaced. A
-- pod of the previous release never reads the column and goes on writing entries as before.
-- +goose Up
ALTER TABLE entry_status_change ADD COLUMN IF NOT EXISTS from_restore boolean NOT NULL DEFAULT false;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION work_item_status_history() RETURNS trigger LANGUAGE plpgsql AS
$$
DECLARE
  entered   timestamptz;
  restoring boolean := coalesce(current_setting('hubtask.restoring', true), '') = 'on';
BEGIN
  IF NEW.status IS NULL THEN
    RETURN NULL;
  END IF;
  IF TG_OP = 'UPDATE' AND NEW.status IS NOT DISTINCT FROM OLD.status THEN
    RETURN NULL;
  END IF;
  IF restoring THEN
    entered := transaction_timestamp();
  ELSIF TG_OP = 'UPDATE' THEN
    entered := NEW.updated_at;
  ELSE
    entered := NEW.created_at;
  END IF;
  INSERT INTO entry_status_change (tenant_id, item_id, status, entered_at, from_restore)
  VALUES (NEW.tenant_id, NEW.id, NEW.status, entered, restoring);
  RETURN NULL;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
