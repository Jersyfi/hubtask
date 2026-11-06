-- A restored entry's status history (UC-GOL-04 check 11). The history is not archived, and a
-- replace-restore deletes the living entries first, so their true history goes with them. The
-- trigger then dated the restored row's status from its `created_at`: an entry created a year ago
-- that entered In Progress yesterday read as In Progress for a year, and cycle time, WIP age and
-- time in status were wrong for every restored entry without a sign.
--
-- While a restore writes (the transaction-local `hubtask.restoring`, set by the import's write of
-- an entry), the moment is the restore's own: the history honestly starts at the restore. Expand
-- only (rule 12): a pod of the previous release never sets the setting and writes as before.
-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION work_item_status_history() RETURNS trigger LANGUAGE plpgsql AS
$$
DECLARE
  entered timestamptz;
BEGIN
  IF NEW.status IS NULL THEN
    RETURN NULL;
  END IF;
  IF TG_OP = 'UPDATE' AND NEW.status IS NOT DISTINCT FROM OLD.status THEN
    RETURN NULL;
  END IF;
  IF coalesce(current_setting('hubtask.restoring', true), '') = 'on' THEN
    entered := transaction_timestamp();
  ELSIF TG_OP = 'UPDATE' THEN
    entered := NEW.updated_at;
  ELSE
    entered := NEW.created_at;
  END IF;
  INSERT INTO entry_status_change (tenant_id, item_id, status, entered_at)
  VALUES (NEW.tenant_id, NEW.id, NEW.status, entered);
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
