-- The slim entry (F-10), the writers' step (versioning-release.md §4): the due announcements'
-- claim and next moment read the stamps from `work_item_announcement` and no longer name the
-- entry's columns, so `wi_due_announce_idx` - partial over those columns - stops matching them.
-- This is the index they walk instead: an open entry with a deadline, by tenant and date. Whether
-- it is still owed an announcement is answered by the side row's primary key.
--
-- Built CONCURRENTLY without a transaction, as 0441; a leftover invalid index is dropped first.
-- `wi_due_announce_idx` goes with the columns in the release after.

-- +goose NO TRANSACTION

-- +goose Up

-- +goose StatementBegin
DO $leftover$
BEGIN
  SET LOCAL lock_timeout = '5s';
  IF EXISTS (SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
             WHERE NOT i.indisvalid AND c.relname = 'wi_due_open_idx') THEN
    DROP INDEX IF EXISTS wi_due_open_idx;
  END IF;
END
$leftover$;
-- +goose StatementEnd

CREATE INDEX CONCURRENTLY IF NOT EXISTS wi_due_open_idx
  ON work_item (tenant_id, due_at)
  WHERE due_at IS NOT NULL AND deleted_at IS NULL AND archived_at IS NULL AND is_completed = false;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
