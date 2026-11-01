-- The side rows (0227) of every entry written before its trigger: a keyset walk over the primary
-- key, one transaction per batch, for the reasons 0212's backfill gives - including the policy a
-- migrator without BYPASSRLS gives itself for the length of the walk, so that it never reports
-- success having written nothing. Only entries that carry one of the mirrored values are read,
-- which is a small share of any workspace.

-- +goose NO TRANSACTION

-- +goose Up
-- The backfills' ledger (versioning-release.md §4): where a walk stopped, one row per migration
-- while it runs, so that a run killed between batches resumes after the last batch it committed
-- rather than from the start. Created by the first backfill that needs it, and owned by the
-- migrator: the application has no grant.
CREATE TABLE IF NOT EXISTS migration_cursor (
  name       text PRIMARY KEY CHECK (name ~ '^[0-9]{4}_[a-z0-9_]{1,80}$'),
  position   text NOT NULL CHECK (char_length(position) <= 200),
  updated_at timestamptz NOT NULL
);
REVOKE ALL ON migration_cursor FROM hubtask_app;

-- +goose StatementBegin
DO $backfill$
DECLARE
  unrestricted boolean;
  batch        uuid[];
  cursor_id    uuid;
BEGIN
  SELECT position::uuid INTO cursor_id FROM migration_cursor WHERE name = '0228_work_item_side_tables_backfill';
  cursor_id := coalesce(cursor_id, '00000000-0000-0000-0000-000000000000');
  SELECT rolsuper OR rolbypassrls INTO unrestricted FROM pg_roles WHERE rolname = current_user;
  IF NOT unrestricted THEN
    EXECUTE 'CREATE POLICY side_rows_backfill ON work_item TO CURRENT_USER USING (true) WITH CHECK (true)';
    EXECUTE 'CREATE POLICY side_rows_backfill ON work_item_announcement TO CURRENT_USER USING (true) WITH CHECK (true)';
    EXECUTE 'CREATE POLICY side_rows_backfill ON work_item_flow TO CURRENT_USER USING (true) WITH CHECK (true)';
    EXECUTE 'CREATE POLICY side_rows_backfill ON work_item_retention TO CURRENT_USER USING (true) WITH CHECK (true)';
  END IF;

  LOOP
    SELECT array_agg(id ORDER BY id) INTO batch
      FROM (SELECT id FROM work_item
             WHERE id > cursor_id
               AND num_nonnulls(due_soon_announced_at, overdue_announced_at, flow_started_at,
                                flow_finished_at, retention_pending_until, retention_rule_id,
                                retention_action, retention_blocked_by) > 0
             ORDER BY id LIMIT 2000) AS page;
    EXIT WHEN batch IS NULL;

    PERFORM work_item_mirror_side_rows(wi) FROM work_item wi WHERE wi.id = ANY(batch);

    cursor_id := batch[array_upper(batch, 1)];
    INSERT INTO migration_cursor (name, position, updated_at)
    VALUES ('0228_work_item_side_tables_backfill', cursor_id::text, clock_timestamp())
    ON CONFLICT (name) DO UPDATE SET position = EXCLUDED.position, updated_at = EXCLUDED.updated_at;
    COMMIT;
  END LOOP;
  DELETE FROM migration_cursor WHERE name = '0228_work_item_side_tables_backfill';

  IF NOT unrestricted THEN
    EXECUTE 'DROP POLICY side_rows_backfill ON work_item';
    EXECUTE 'DROP POLICY side_rows_backfill ON work_item_announcement';
    EXECUTE 'DROP POLICY side_rows_backfill ON work_item_flow';
    EXECUTE 'DROP POLICY side_rows_backfill ON work_item_retention';
  END IF;
END $backfill$;
-- +goose StatementEnd

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
