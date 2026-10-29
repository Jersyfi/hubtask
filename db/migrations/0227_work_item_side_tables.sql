-- The slim entry (F-10), expand half (versioning-release.md §4): what a background loop keeps on an
-- entry - when its due date was announced, its flow moments, what a retention run marked on it -
-- moves to three side tables keyed by the entry, so that the row every list, board and sync read
-- carries only what a person wrote.
--
-- This release writes both: the columns on `work_item` stay the ones every writer and reader uses,
-- and a trigger mirrors them into the side tables in the writing transaction, by every path (use
-- cases, sync, restore, import, an older binary in a rolling update). A side row exists only while
-- one of its values does, so the tables stay as sparse as the facts. The backfill follows in 0228,
-- outside a transaction. Readers move to the side tables, and the columns are dropped, in the
-- release after the suite (the contract half).
--
-- Rule 3 holds as for every tenant table: forced row level security on `tenant_id`.
-- +goose Up
CREATE TABLE IF NOT EXISTS work_item_announcement (
  tenant_id             uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id               uuid NOT NULL,
  due_soon_announced_at timestamptz,
  overdue_announced_at  timestamptz,
  PRIMARY KEY (tenant_id, item_id),
  CHECK (num_nonnulls(due_soon_announced_at, overdue_announced_at) > 0),
  CONSTRAINT work_item_announcement_item_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS work_item_flow (
  tenant_id        uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id          uuid NOT NULL,
  flow_started_at  timestamptz,
  flow_finished_at timestamptz,
  PRIMARY KEY (tenant_id, item_id),
  CHECK (num_nonnulls(flow_started_at, flow_finished_at) > 0),
  CONSTRAINT work_item_flow_item_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS work_item_retention (
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id       uuid NOT NULL,
  pending_until timestamptz,
  rule_id       uuid,
  action        text,
  blocked_by    text,
  PRIMARY KEY (tenant_id, item_id),
  CHECK (num_nonnulls(pending_until, rule_id, action, blocked_by) > 0),
  CONSTRAINT work_item_retention_item_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
-- Phase two's walk (data-retention.md §6) reads the due moment, as work_item_retention_idx does.
CREATE INDEX IF NOT EXISTS work_item_retention_due_idx
  ON work_item_retention (tenant_id, pending_until) WHERE pending_until IS NOT NULL;

ALTER TABLE work_item_announcement ENABLE ROW LEVEL SECURITY;
ALTER TABLE work_item_announcement FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON work_item_announcement;
CREATE POLICY tenant_isolation ON work_item_announcement
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE work_item_flow ENABLE ROW LEVEL SECURITY;
ALTER TABLE work_item_flow FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON work_item_flow;
CREATE POLICY tenant_isolation ON work_item_flow
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE work_item_retention ENABLE ROW LEVEL SECURITY;
ALTER TABLE work_item_retention FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON work_item_retention;
CREATE POLICY tenant_isolation ON work_item_retention
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

GRANT SELECT, INSERT, UPDATE, DELETE ON work_item_announcement, work_item_flow, work_item_retention
  TO hubtask_app;

-- One entry's three side rows from its columns. Shared by the trigger and the backfill (0228), so
-- the two cannot disagree about when a row exists.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION work_item_mirror_side_rows(entry work_item) RETURNS void
  LANGUAGE plpgsql AS
$$
BEGIN
  IF num_nonnulls(entry.due_soon_announced_at, entry.overdue_announced_at) > 0 THEN
    INSERT INTO work_item_announcement (tenant_id, item_id, due_soon_announced_at, overdue_announced_at)
    VALUES (entry.tenant_id, entry.id, entry.due_soon_announced_at, entry.overdue_announced_at)
    ON CONFLICT (tenant_id, item_id) DO UPDATE
      SET due_soon_announced_at = EXCLUDED.due_soon_announced_at,
          overdue_announced_at  = EXCLUDED.overdue_announced_at;
  ELSE
    DELETE FROM work_item_announcement WHERE tenant_id = entry.tenant_id AND item_id = entry.id;
  END IF;

  IF num_nonnulls(entry.flow_started_at, entry.flow_finished_at) > 0 THEN
    INSERT INTO work_item_flow (tenant_id, item_id, flow_started_at, flow_finished_at)
    VALUES (entry.tenant_id, entry.id, entry.flow_started_at, entry.flow_finished_at)
    ON CONFLICT (tenant_id, item_id) DO UPDATE
      SET flow_started_at  = EXCLUDED.flow_started_at,
          flow_finished_at = EXCLUDED.flow_finished_at;
  ELSE
    DELETE FROM work_item_flow WHERE tenant_id = entry.tenant_id AND item_id = entry.id;
  END IF;

  IF num_nonnulls(entry.retention_pending_until, entry.retention_rule_id, entry.retention_action,
                  entry.retention_blocked_by) > 0 THEN
    INSERT INTO work_item_retention (tenant_id, item_id, pending_until, rule_id, action, blocked_by)
    VALUES (entry.tenant_id, entry.id, entry.retention_pending_until, entry.retention_rule_id,
            entry.retention_action, entry.retention_blocked_by)
    ON CONFLICT (tenant_id, item_id) DO UPDATE
      SET pending_until = EXCLUDED.pending_until,
          rule_id       = EXCLUDED.rule_id,
          action        = EXCLUDED.action,
          blocked_by    = EXCLUDED.blocked_by;
  ELSE
    DELETE FROM work_item_retention WHERE tenant_id = entry.tenant_id AND item_id = entry.id;
  END IF;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION work_item_side_rows() RETURNS trigger LANGUAGE plpgsql AS
$$
BEGIN
  -- An insert that carries none of the mirrored values - nearly every one - writes nothing.
  IF TG_OP = 'INSERT'
     AND num_nonnulls(NEW.due_soon_announced_at, NEW.overdue_announced_at, NEW.flow_started_at,
                      NEW.flow_finished_at, NEW.retention_pending_until, NEW.retention_rule_id,
                      NEW.retention_action, NEW.retention_blocked_by) = 0 THEN
    RETURN NULL;
  END IF;
  IF TG_OP = 'UPDATE'
     AND (NEW.due_soon_announced_at, NEW.overdue_announced_at, NEW.flow_started_at,
          NEW.flow_finished_at, NEW.retention_pending_until, NEW.retention_rule_id,
          NEW.retention_action, NEW.retention_blocked_by)
         IS NOT DISTINCT FROM
         (OLD.due_soon_announced_at, OLD.overdue_announced_at, OLD.flow_started_at,
          OLD.flow_finished_at, OLD.retention_pending_until, OLD.retention_rule_id,
          OLD.retention_action, OLD.retention_blocked_by) THEN
    RETURN NULL;
  END IF;
  PERFORM work_item_mirror_side_rows(NEW);
  RETURN NULL;
END
$$;
-- +goose StatementEnd

-- AFTER, so the flow moments the BEFORE trigger work_item_flow_moments (0194) set are the ones
-- mirrored. `bucket_id` and `collection_id` are in the list because that trigger writes the
-- moments on a move, and an UPDATE OF list sees only the statement's own SET columns.
DROP TRIGGER IF EXISTS work_item_side_rows ON work_item;
CREATE TRIGGER work_item_side_rows
  AFTER INSERT OR UPDATE OF due_soon_announced_at, overdue_announced_at, flow_started_at,
    flow_finished_at, retention_pending_until, retention_rule_id, retention_action,
    retention_blocked_by, bucket_id, collection_id
  ON work_item
  FOR EACH ROW EXECUTE FUNCTION work_item_side_rows();

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
