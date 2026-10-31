-- The field kinds a register and a tracker add (F-2, UC-WRK-12): DATE_TIME, GROUP,
-- CASCADING_SELECT, IP_ADDRESS, FILE, LABEL and STATUS.
--
-- The projection into `entry_field_value` gives a moment the `ts` column, a group and a file the
-- `ref` column, a two-level choice, a label and a state the `text` column. An address lands in
-- `text` in its canonical spelling, as equality and STARTS_WITH compare it, and in the new `addr`
-- column as `inet`, which ranges and sorts by the address rather than by its spelling. The row's
-- one-typed-value CHECK stays as it is: an address's row holds its text as well, so `addr` is an
-- addition beside it, never a fifth column the CHECK would have to admit.
--
-- Expand only (versioning-release.md §4): the kind CHECK widens, `addr` is new and nullable, and an
-- older binary defines none of the new kinds. The index on `addr` is built CONCURRENTLY in 0299.

-- +goose Up
ALTER TABLE custom_field_definition DROP CONSTRAINT IF EXISTS custom_field_definition_kind_check;
ALTER TABLE custom_field_definition ADD CONSTRAINT custom_field_definition_kind_check
  CHECK (kind IN ('TEXT','NUMBER','DATE','SELECT','MULTI_SELECT','BOOL','USER','URL',
                  'MONEY','DURATION','REFERENCE','USER_SET','RICH_TEXT','CALCULATED',
                  'DATE_TIME','GROUP','CASCADING_SELECT','IP_ADDRESS','FILE','LABEL','STATUS'));

ALTER TABLE entry_field_value ADD COLUMN IF NOT EXISTS addr inet;
ALTER TABLE entry_field_value DROP CONSTRAINT IF EXISTS entry_field_value_addr_check;
ALTER TABLE entry_field_value ADD CONSTRAINT entry_field_value_addr_check
  CHECK (addr IS NULL OR text IS NOT NULL) NOT VALID;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION entry_field_values_of(entry work_item) RETURNS SETOF entry_field_value
  LANGUAGE sql STABLE AS
$$
  SELECT entry.tenant_id, entry.id, d.id, (e.ordinal - 1)::integer,
         CASE
           WHEN d.kind IN ('NUMBER', 'MONEY', 'DURATION', 'CALCULATED') AND jsonb_typeof(e.value) = 'number'
             THEN (e.value #>> '{}')::numeric
           WHEN d.kind = 'BOOL' AND jsonb_typeof(e.value) = 'boolean'
             THEN CASE WHEN (e.value #>> '{}')::boolean THEN 1 ELSE 0 END
         END,
         CASE
           WHEN d.kind = 'DATE' AND (e.value #>> '{}') ~ '^\d{4}-\d{2}-\d{2}$'
             THEN ((e.value #>> '{}')::date)::timestamp AT TIME ZONE 'UTC'
           WHEN d.kind = 'DATE_TIME' AND (e.value #>> '{}') ~ '^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$'
             THEN (e.value #>> '{}')::timestamptz
         END,
         CASE
           WHEN d.kind IN ('TEXT', 'SELECT', 'MULTI_SELECT', 'URL', 'CASCADING_SELECT', 'LABEL', 'STATUS')
            AND jsonb_typeof(e.value) = 'string'
             THEN left(e.value #>> '{}', 256)
           WHEN d.kind = 'IP_ADDRESS' AND jsonb_typeof(e.value) = 'string' AND pg_input_is_valid(e.value #>> '{}', 'inet')
             THEN left(e.value #>> '{}', 256)
         END,
         CASE
           WHEN d.kind IN ('USER', 'USER_SET', 'REFERENCE', 'GROUP', 'FILE')
            AND (e.value #>> '{}') ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
             THEN (e.value #>> '{}')::uuid
         END,
         CASE
           WHEN d.kind = 'IP_ADDRESS' AND jsonb_typeof(e.value) = 'string' AND pg_input_is_valid(e.value #>> '{}', 'inet')
             THEN (e.value #>> '{}')::inet
         END
    FROM jsonb_each(entry.custom_fields) AS kv
    JOIN custom_field_definition d
      ON d.tenant_id = entry.tenant_id
     AND d.id::text = jsonb_extract_path_text(entry.custom_field_refs, kv.key)
    CROSS JOIN LATERAL jsonb_array_elements(
      CASE WHEN jsonb_typeof(kv.value) = 'array' THEN kv.value ELSE jsonb_build_array(kv.value) END
    ) WITH ORDINALITY AS e(value, ordinal)
   WHERE d.kind <> 'RICH_TEXT' AND jsonb_typeof(e.value) <> 'null'
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION work_item_field_values() RETURNS trigger LANGUAGE plpgsql AS
$$
BEGIN
  IF TG_OP = 'UPDATE'
     AND NEW.custom_fields IS NOT DISTINCT FROM OLD.custom_fields
     AND NEW.custom_field_refs IS NOT DISTINCT FROM OLD.custom_field_refs THEN
    RETURN NULL;
  END IF;
  IF TG_OP = 'UPDATE' THEN
    DELETE FROM entry_field_value WHERE tenant_id = NEW.tenant_id AND entry_id = NEW.id;
  END IF;
  INSERT INTO entry_field_value (tenant_id, entry_id, definition_id, ordinal, num, ts, text, ref, addr)
  SELECT v.tenant_id, v.entry_id, v.definition_id, v.ordinal, v.num, v.ts, v.text, v.ref, v.addr
    FROM entry_field_values_of(NEW) AS v
   WHERE num_nonnulls(v.num, v.ts, v.text, v.ref) = 1;
  RETURN NULL;
END
$$;
-- +goose StatementEnd

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
