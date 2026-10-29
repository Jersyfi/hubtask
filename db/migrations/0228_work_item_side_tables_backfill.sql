-- The side rows (0227) of every entry written before its trigger: a keyset walk over the primary
-- key, one transaction per batch, for the reasons 0212's backfill gives - including the policy a
-- migrator without BYPASSRLS gives itself for the length of the walk, so that it never reports
-- success having written nothing. Only entries that carry one of the mirrored values are read,
-- which is a small share of any workspace.

-- +goose NO TRANSACTION

-- +goose Up
-- +goose StatementBegin
DO $backfill$
DECLARE
  unrestricted boolean;
  batch        uuid[];
  cursor_id    uuid := '00000000-0000-0000-0000-000000000000';
BEGIN
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
    COMMIT;
  END LOOP;

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
