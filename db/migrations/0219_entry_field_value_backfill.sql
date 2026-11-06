-- The typed values of every entry written before 0218's trigger: a keyset walk over the primary
-- key, one transaction per batch, for the reasons 0019's backfill gives - including the policy a
-- migrator without BYPASSRLS gives itself for the length of the walk, on the three tables it reads
-- and writes, so that it never reports success having written nothing.

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
  SELECT position::uuid INTO cursor_id FROM migration_cursor WHERE name = '0219_entry_field_value_backfill';
  cursor_id := coalesce(cursor_id, '00000000-0000-0000-0000-000000000000');
  SELECT rolsuper OR rolbypassrls INTO unrestricted FROM pg_roles WHERE rolname = current_user;
  IF NOT unrestricted THEN
    EXECUTE 'CREATE POLICY field_value_backfill ON work_item TO CURRENT_USER USING (true) WITH CHECK (true)';
    EXECUTE 'CREATE POLICY field_value_backfill ON custom_field_definition TO CURRENT_USER USING (true) WITH CHECK (true)';
    EXECUTE 'CREATE POLICY field_value_backfill ON entry_field_value TO CURRENT_USER USING (true) WITH CHECK (true)';
  END IF;

  LOOP
    SELECT array_agg(id ORDER BY id) INTO batch
      FROM (SELECT id FROM work_item WHERE id > cursor_id AND custom_fields <> '{}'::jsonb
            ORDER BY id LIMIT 2000) AS page;
    EXIT WHEN batch IS NULL;

    INSERT INTO entry_field_value (tenant_id, entry_id, definition_id, ordinal, num, ts, text, ref)
    SELECT v.tenant_id, v.entry_id, v.definition_id, v.ordinal, v.num, v.ts, v.text, v.ref
      FROM work_item wi
      CROSS JOIN LATERAL entry_field_values_of(wi) AS v
     WHERE wi.id = ANY(batch) AND num_nonnulls(v.num, v.ts, v.text, v.ref) = 1
    ON CONFLICT DO NOTHING;

    cursor_id := batch[array_upper(batch, 1)];
    INSERT INTO migration_cursor (name, position, updated_at)
    VALUES ('0219_entry_field_value_backfill', cursor_id::text, clock_timestamp())
    ON CONFLICT (name) DO UPDATE SET position = EXCLUDED.position, updated_at = EXCLUDED.updated_at;
    COMMIT;
  END LOOP;
  DELETE FROM migration_cursor WHERE name = '0219_entry_field_value_backfill';

  IF NOT unrestricted THEN
    EXECUTE 'DROP POLICY field_value_backfill ON work_item';
    EXECUTE 'DROP POLICY field_value_backfill ON custom_field_definition';
    EXECUTE 'DROP POLICY field_value_backfill ON entry_field_value';
  END IF;
END $backfill$;
-- +goose StatementEnd

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
