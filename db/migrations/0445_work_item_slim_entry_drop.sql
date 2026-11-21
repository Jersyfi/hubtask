-- The slim entry (F-10), the drop (versioning-release.md §4, "Scheduled contract steps"): since
-- v0.10.1 no binary reads or writes the entry's eight columns, and the side tables of 0234 are the
-- only place their values live. What kept the columns and the side rows in step for v0.10.0 goes:
-- the mirror `work_item_side_rows` and its function `work_item_mirror_side_rows`, and the
-- `work_item_columns_follow` triggers of 0443 with their functions. Then the columns, the two
-- partial indexes over them, and the generated `search_vector`, which the search stopped reading
-- with `search_document` (0019, ADR-0034).
--
-- The flow moments' trigger of 0443 stays and is replaced without the branches that left a
-- statement naming the columns to the mirror: no writer names them, and after this none can.
--
-- One transaction, so no write sees the columns gone and the triggers that name them still in
-- place. Every step is a catalogue change: the ACCESS EXCLUSIVE lock on `work_item` is held for
-- as long as the catalogue takes, not for a scan, and `lock_timeout` bounds the wait for it.
--
-- The compatibility floor rises to 0442, the first migration of v0.10.1: a binary below it names
-- columns this migration drops, and is not ready rather than failing every write (deployment.md
-- §5).
-- +goose Up

DROP TRIGGER IF EXISTS work_item_side_rows ON work_item;
DROP FUNCTION IF EXISTS work_item_side_rows();
DROP FUNCTION IF EXISTS work_item_mirror_side_rows(work_item);

DROP TRIGGER IF EXISTS work_item_columns_follow ON work_item_announcement;
DROP TRIGGER IF EXISTS work_item_columns_follow ON work_item_retention;
DROP FUNCTION IF EXISTS work_item_announcement_follow();
DROP FUNCTION IF EXISTS work_item_retention_follow();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION work_item_flow_side_row() RETURNS trigger LANGUAGE plpgsql AS
$$
DECLARE
  moment timestamptz;
  first_column uuid;
  completes boolean;
  had_start timestamptz;
  had_finish timestamptz;
  v_start timestamptz;
  v_finish timestamptz;
BEGIN
  IF TG_OP = 'UPDATE' THEN
    IF NEW.collection_id IS NOT DISTINCT FROM OLD.collection_id
       AND NEW.bucket_id IS NOT DISTINCT FROM OLD.bucket_id THEN
      RETURN NULL;
    END IF;
    moment := NEW.updated_at;
  ELSE
    moment := NEW.created_at;
  END IF;

  SELECT f.flow_started_at, f.flow_finished_at INTO had_start, had_finish
    FROM work_item_flow f WHERE f.tenant_id = NEW.tenant_id AND f.item_id = NEW.id FOR UPDATE;
  IF TG_OP = 'UPDATE' AND NEW.collection_id IS DISTINCT FROM OLD.collection_id THEN
    -- Another board: its first column is another one.
    v_start := NULL;
    v_finish := NULL;
  ELSE
    v_start := had_start;
    v_finish := had_finish;
  END IF;

  IF NEW.bucket_id IS NOT NULL THEN
    -- The board's first column as a board is answered: by rank, then identifier (ListBuckets).
    SELECT b.id INTO first_column
      FROM bucket b
     WHERE b.tenant_id = NEW.tenant_id AND b.collection_id = NEW.collection_id AND b.deleted_at IS NULL
     ORDER BY b.order_key COLLATE "C", b.id
     LIMIT 1;
    SELECT b.is_done_bucket INTO completes
      FROM bucket b
     WHERE b.tenant_id = NEW.tenant_id AND b.id = NEW.bucket_id;

    IF NEW.bucket_id = first_column AND v_start IS NULL THEN
      v_start := moment;
    END IF;
    IF completes AND (TG_OP = 'UPDATE' OR v_finish IS NULL) THEN
      v_finish := moment;
    END IF;
  END IF;

  IF (v_start, v_finish) IS NOT DISTINCT FROM (had_start, had_finish) THEN
    RETURN NULL;
  END IF;
  IF num_nonnulls(v_start, v_finish) = 0 THEN
    DELETE FROM work_item_flow WHERE tenant_id = NEW.tenant_id AND item_id = NEW.id;
  ELSE
    INSERT INTO work_item_flow (tenant_id, item_id, flow_started_at, flow_finished_at)
    VALUES (NEW.tenant_id, NEW.id, v_start, v_finish)
    ON CONFLICT (tenant_id, item_id) DO UPDATE
      SET flow_started_at  = EXCLUDED.flow_started_at,
          flow_finished_at = EXCLUDED.flow_finished_at;
  END IF;
  RETURN NULL;
END
$$;
-- +goose StatementEnd

DROP INDEX IF EXISTS wi_due_announce_idx;
DROP INDEX IF EXISTS work_item_retention_idx;
DROP INDEX IF EXISTS wi_search_idx;

ALTER TABLE work_item
  DROP COLUMN IF EXISTS due_soon_announced_at,
  DROP COLUMN IF EXISTS overdue_announced_at,
  DROP COLUMN IF EXISTS flow_started_at,
  DROP COLUMN IF EXISTS flow_finished_at,
  DROP COLUMN IF EXISTS retention_pending_until,
  DROP COLUMN IF EXISTS retention_rule_id,
  DROP COLUMN IF EXISTS retention_action,
  DROP COLUMN IF EXISTS retention_blocked_by,
  DROP COLUMN IF EXISTS search_vector;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION schema_compatibility_floor()
RETURNS bigint
LANGUAGE sql IMMUTABLE SET search_path = pg_catalog, pg_temp AS $$
  SELECT 442::bigint;
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
