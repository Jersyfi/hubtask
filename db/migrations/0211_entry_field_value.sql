-- Typed fields (F-2, UC-WRK-12): five more kinds, and a typed side table the query language ranges,
-- sorts and groups over.
--
-- `work_item.custom_fields` stays the document every read answers with; `entry_field_value` is
-- its typed projection - one row per value (per element of a set) with the value in the one
-- column its kind compares in: `num` for NUMBER, MONEY, DURATION and BOOL (1 or 0), `ts` for DATE
-- (midnight UTC of the day), `text` for TEXT, SELECT, MULTI_SELECT and URL (its first 256
-- characters, the most a B-tree entry may carry in every script), `ref` for USER, USER_SET and
-- REFERENCE. RICH_TEXT has no row: prose is not compared.
--
-- A trigger writes it, so it is written in the transaction that writes the document, by every
-- path - use cases, sync, restore, import - exactly as the search document is (migration 0019).
-- The definition a value was written under is `custom_field_refs`' answer, so a value under a
-- deleted definition keeps its row and is hidden by the query's live-definition check, as the
-- document's value is.
--
-- Expand only (versioning-release.md §4): the CHECK widens, the currency column is new and
-- nullable, and an older binary writes none of the new kinds. The backfill of today's values
-- follows in 0212, outside a transaction.

-- +goose Up
ALTER TABLE custom_field_definition DROP CONSTRAINT IF EXISTS custom_field_definition_kind_check;
ALTER TABLE custom_field_definition ADD CONSTRAINT custom_field_definition_kind_check
  CHECK (kind IN ('TEXT','NUMBER','DATE','SELECT','MULTI_SELECT','BOOL','USER','URL',
                  'MONEY','DURATION','REFERENCE','USER_SET','RICH_TEXT'));

ALTER TABLE custom_field_definition ADD COLUMN IF NOT EXISTS currency text;
ALTER TABLE custom_field_definition DROP CONSTRAINT IF EXISTS custom_field_definition_currency_check;
ALTER TABLE custom_field_definition ADD CONSTRAINT custom_field_definition_currency_check
  CHECK ((kind = 'MONEY') = (currency IS NOT NULL) AND (currency IS NULL OR currency ~ '^[A-Z]{3}$'));

CREATE TABLE IF NOT EXISTS entry_field_value (
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  entry_id      uuid NOT NULL,
  definition_id uuid NOT NULL,
  -- The element's place in a set; 0 for a single value.
  ordinal       integer NOT NULL DEFAULT 0 CHECK (ordinal >= 0),
  num           numeric,
  ts            timestamptz,
  text          text CHECK (text IS NULL OR char_length(text) <= 256),
  ref           uuid,
  PRIMARY KEY (tenant_id, entry_id, definition_id, ordinal),
  CHECK (num_nonnulls(num, ts, text, ref) = 1),
  CONSTRAINT entry_field_value_entry_fkey
    FOREIGN KEY (tenant_id, entry_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT entry_field_value_definition_fkey
    FOREIGN KEY (tenant_id, definition_id) REFERENCES custom_field_definition (tenant_id, id)
    ON DELETE CASCADE
);
-- Range, sort and aggregation per field (F-2). Empty at creation, so built inside the transaction.
CREATE INDEX IF NOT EXISTS efv_num_idx ON entry_field_value (tenant_id, definition_id, num)
  WHERE num IS NOT NULL;
CREATE INDEX IF NOT EXISTS efv_ts_idx ON entry_field_value (tenant_id, definition_id, ts)
  WHERE ts IS NOT NULL;
CREATE INDEX IF NOT EXISTS efv_text_idx ON entry_field_value (tenant_id, definition_id, text)
  WHERE text IS NOT NULL;
CREATE INDEX IF NOT EXISTS efv_ref_idx ON entry_field_value (tenant_id, definition_id, ref)
  WHERE ref IS NOT NULL;

ALTER TABLE entry_field_value ENABLE ROW LEVEL SECURITY;
ALTER TABLE entry_field_value FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON entry_field_value;
CREATE POLICY tenant_isolation ON entry_field_value
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON entry_field_value TO hubtask_app;

-- The projection of one entry's document. Shared by the trigger and the backfill (0212), so the
-- two cannot disagree about which column a kind lands in.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION entry_field_values_of(entry work_item) RETURNS SETOF entry_field_value
  LANGUAGE sql STABLE AS
$$
  SELECT entry.tenant_id, entry.id, d.id, (e.ordinal - 1)::integer,
         CASE
           WHEN d.kind IN ('NUMBER', 'MONEY', 'DURATION') AND jsonb_typeof(e.value) = 'number'
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
  INSERT INTO entry_field_value (tenant_id, entry_id, definition_id, ordinal, num, ts, text, ref)
  SELECT v.tenant_id, v.entry_id, v.definition_id, v.ordinal, v.num, v.ts, v.text, v.ref
    FROM entry_field_values_of(NEW) AS v
   WHERE num_nonnulls(v.num, v.ts, v.text, v.ref) = 1;
  RETURN NULL;
END
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS work_item_field_values ON work_item;
CREATE TRIGGER work_item_field_values
  AFTER INSERT OR UPDATE OF custom_fields, custom_field_refs ON work_item
  FOR EACH ROW EXECUTE FUNCTION work_item_field_values();

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
