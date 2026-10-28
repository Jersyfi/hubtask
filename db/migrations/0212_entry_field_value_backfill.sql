-- The typed values of every entry written before 0211's trigger: a keyset walk over the primary
-- key, one transaction per batch, for the reasons 0019's backfill gives - including the policy a
-- migrator without BYPASSRLS gives itself for the length of the walk, on the three tables it reads
-- and writes, so that it never reports success having written nothing.

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
    COMMIT;
  END LOOP;

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
