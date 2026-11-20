-- The slim entry (F-10), the writers' step (versioning-release.md §4): from this release every
-- writer writes the side tables of 0234 directly and none names the entry's eight columns, which
-- therefore go stale. Two pieces of the database still worked from the columns and are moved here.
--
-- The mirror `work_item_side_rows` stays for the older binary still rolling out, which writes the
-- columns. It wrote a whole side row from the columns whenever any of the eight changed - from
-- stale columns that would undo what this release wrote: a card moved into its first column
-- changed `flow_started_at`, and the mirror then deleted the announcement this release had claimed
-- (the entry was announced twice) and the retention mark it had set. Now it carries only the
-- columns a statement changed, onto the side row as it stands.
--
-- The flow moments (0194) were set on the entry's columns by a BEFORE trigger and read back from
-- them: an entry whose moments a restore wrote to the side row would have its start recorded
-- again on its next move. The trigger now runs AFTER and keeps `work_item_flow` itself, with
-- 0194's rules unchanged; a statement of the older binary that writes the moments itself is left
-- to the mirror, as 0194 left it to the statement.
--
-- Both are replaced in one transaction, so no write runs between the two flow triggers.
-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION work_item_side_rows() RETURNS trigger LANGUAGE plpgsql AS
$$
DECLARE
  inserting boolean := TG_OP = 'INSERT';
  c_soon boolean; c_over boolean; c_start boolean; c_finish boolean;
  c_until boolean; c_rule boolean; c_action boolean; c_blocked boolean;
  v_soon timestamptz; v_over timestamptz; v_start timestamptz; v_finish timestamptz;
  v_until timestamptz; v_rule uuid; v_action text; v_blocked text;
BEGIN
  IF inserting THEN
    c_soon := NEW.due_soon_announced_at IS NOT NULL;
    c_over := NEW.overdue_announced_at IS NOT NULL;
    c_start := NEW.flow_started_at IS NOT NULL;
    c_finish := NEW.flow_finished_at IS NOT NULL;
    c_until := NEW.retention_pending_until IS NOT NULL;
    c_rule := NEW.retention_rule_id IS NOT NULL;
    c_action := NEW.retention_action IS NOT NULL;
    c_blocked := NEW.retention_blocked_by IS NOT NULL;
  ELSE
    c_soon := NEW.due_soon_announced_at IS DISTINCT FROM OLD.due_soon_announced_at;
    c_over := NEW.overdue_announced_at IS DISTINCT FROM OLD.overdue_announced_at;
    c_start := NEW.flow_started_at IS DISTINCT FROM OLD.flow_started_at;
    c_finish := NEW.flow_finished_at IS DISTINCT FROM OLD.flow_finished_at;
    c_until := NEW.retention_pending_until IS DISTINCT FROM OLD.retention_pending_until;
    c_rule := NEW.retention_rule_id IS DISTINCT FROM OLD.retention_rule_id;
    c_action := NEW.retention_action IS DISTINCT FROM OLD.retention_action;
    c_blocked := NEW.retention_blocked_by IS DISTINCT FROM OLD.retention_blocked_by;
  END IF;

  IF c_soon OR c_over THEN
    SELECT a.due_soon_announced_at, a.overdue_announced_at INTO v_soon, v_over
      FROM work_item_announcement a WHERE a.tenant_id = NEW.tenant_id AND a.item_id = NEW.id FOR UPDATE;
    IF c_soon THEN v_soon := NEW.due_soon_announced_at; END IF;
    IF c_over THEN v_over := NEW.overdue_announced_at; END IF;
    IF num_nonnulls(v_soon, v_over) = 0 THEN
      DELETE FROM work_item_announcement WHERE tenant_id = NEW.tenant_id AND item_id = NEW.id;
    ELSE
      INSERT INTO work_item_announcement (tenant_id, item_id, due_soon_announced_at, overdue_announced_at)
      VALUES (NEW.tenant_id, NEW.id, v_soon, v_over)
      ON CONFLICT (tenant_id, item_id) DO UPDATE
        SET due_soon_announced_at = EXCLUDED.due_soon_announced_at,
            overdue_announced_at  = EXCLUDED.overdue_announced_at;
    END IF;
  END IF;

  IF c_start OR c_finish THEN
    SELECT f.flow_started_at, f.flow_finished_at INTO v_start, v_finish
      FROM work_item_flow f WHERE f.tenant_id = NEW.tenant_id AND f.item_id = NEW.id FOR UPDATE;
    IF c_start THEN v_start := NEW.flow_started_at; END IF;
    IF c_finish THEN v_finish := NEW.flow_finished_at; END IF;
    IF num_nonnulls(v_start, v_finish) = 0 THEN
      DELETE FROM work_item_flow WHERE tenant_id = NEW.tenant_id AND item_id = NEW.id;
    ELSE
      INSERT INTO work_item_flow (tenant_id, item_id, flow_started_at, flow_finished_at)
      VALUES (NEW.tenant_id, NEW.id, v_start, v_finish)
      ON CONFLICT (tenant_id, item_id) DO UPDATE
        SET flow_started_at  = EXCLUDED.flow_started_at,
            flow_finished_at = EXCLUDED.flow_finished_at;
    END IF;
  END IF;

  IF c_until OR c_rule OR c_action OR c_blocked THEN
    SELECT r.pending_until, r.rule_id, r.action, r.blocked_by INTO v_until, v_rule, v_action, v_blocked
      FROM work_item_retention r WHERE r.tenant_id = NEW.tenant_id AND r.item_id = NEW.id FOR UPDATE;
    IF c_until THEN v_until := NEW.retention_pending_until; END IF;
    IF c_rule THEN v_rule := NEW.retention_rule_id; END IF;
    IF c_action THEN v_action := NEW.retention_action; END IF;
    IF c_blocked THEN v_blocked := NEW.retention_blocked_by; END IF;
    IF num_nonnulls(v_until, v_rule, v_action, v_blocked) = 0 THEN
      DELETE FROM work_item_retention WHERE tenant_id = NEW.tenant_id AND item_id = NEW.id;
    ELSE
      INSERT INTO work_item_retention (tenant_id, item_id, pending_until, rule_id, action, blocked_by)
      VALUES (NEW.tenant_id, NEW.id, v_until, v_rule, v_action, v_blocked)
      ON CONFLICT (tenant_id, item_id) DO UPDATE
        SET pending_until = EXCLUDED.pending_until,
            rule_id       = EXCLUDED.rule_id,
            action        = EXCLUDED.action,
            blocked_by    = EXCLUDED.blocked_by;
    END IF;
  END IF;
  RETURN NULL;
END
$$;
-- +goose StatementEnd

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
    IF NEW.flow_started_at IS DISTINCT FROM OLD.flow_started_at
       OR NEW.flow_finished_at IS DISTINCT FROM OLD.flow_finished_at THEN
      -- The older binary's restore wrote the moments itself; the mirror carries them.
      RETURN NULL;
    END IF;
    IF NEW.collection_id IS NOT DISTINCT FROM OLD.collection_id
       AND NEW.bucket_id IS NOT DISTINCT FROM OLD.bucket_id THEN
      RETURN NULL;
    END IF;
    moment := NEW.updated_at;
  ELSE
    IF num_nonnulls(NEW.flow_started_at, NEW.flow_finished_at) > 0 THEN
      RETURN NULL;
    END IF;
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

DROP TRIGGER IF EXISTS work_item_flow_moments ON work_item;
CREATE TRIGGER work_item_flow_moments
  AFTER INSERT OR UPDATE OF bucket_id, collection_id ON work_item
  FOR EACH ROW EXECUTE FUNCTION work_item_flow_side_row();
DROP FUNCTION IF EXISTS work_item_flow_moments();

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
