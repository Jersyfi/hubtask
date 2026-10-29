-- A calculated field (F-2, UC-WRK-12, domain-model.md §3.5): a NUMBER the server computes from the
-- entry's own fields with a formula, recomputed by every writer of the entry's values. Expand only
-- (versioning-release.md §4): the kind CHECK widens, the formula column is new and nullable and
-- paired with the kind, and an older binary writes no calculated definition. The projection into
-- `entry_field_value` gives the kind the numeric column, as it does NUMBER.

-- +goose Up
ALTER TABLE custom_field_definition DROP CONSTRAINT IF EXISTS custom_field_definition_kind_check;
ALTER TABLE custom_field_definition ADD CONSTRAINT custom_field_definition_kind_check
  CHECK (kind IN ('TEXT','NUMBER','DATE','SELECT','MULTI_SELECT','BOOL','USER','URL',
                  'MONEY','DURATION','REFERENCE','USER_SET','RICH_TEXT','CALCULATED'));

ALTER TABLE custom_field_definition ADD COLUMN IF NOT EXISTS formula text;
ALTER TABLE custom_field_definition DROP CONSTRAINT IF EXISTS custom_field_definition_formula_check;
ALTER TABLE custom_field_definition ADD CONSTRAINT custom_field_definition_formula_check
  CHECK ((kind = 'CALCULATED') = (formula IS NOT NULL) AND (formula IS NULL OR length(formula) <= 2000));

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
         END,
         CASE
           WHEN d.kind IN ('TEXT', 'SELECT', 'MULTI_SELECT', 'URL') AND jsonb_typeof(e.value) = 'string'
             THEN left(e.value #>> '{}', 256)
         END,
         CASE
           WHEN d.kind IN ('USER', 'USER_SET', 'REFERENCE')
            AND (e.value #>> '{}') ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
             THEN (e.value #>> '{}')::uuid
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

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
