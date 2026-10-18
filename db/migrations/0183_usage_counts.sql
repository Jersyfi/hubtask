-- Use the platform can bill by (multi-tenancy.md §4.3, UC-INS-14 checks 4 and 8).
--
-- Each workspace's entries, stored bytes and people are kept per UTC day as the day's highest count
-- (`value`) and its closing count (`closing`), written where the count changes: statement-level
-- triggers add each statement's delta from its transition tables under the day row's lock, and the
-- first write to a count on a day counts afresh, so a drift - an invitation whose link ran out
-- changes the count with no write at all - lasts until that write on a later day. No job visits
-- workspaces. The day row is one per workspace and count, so the writers of one workspace wait on it
-- in turn until they commit. The counts are recorded
-- always; offering them for billing is the operator's switch, read by the application (Decision 18).
--
-- A trigger that failed would fail the write it rides on, so every path that cannot attribute a row
-- records nothing: no workspace in scope (a restore loaded as the owner, the system scope), or a
-- workspace whose purge has begun - the purge moves its counts to `usage_archive` first.
--
-- `usage_archive` is the installation's ledger of purged workspaces' counts, keyed by the bare id:
-- no row policy and no foreign key, like `instance_event`, because its rows outlive the workspace.
-- The read and the sweep are SECURITY DEFINER functions through the owner's own policies on
-- `usage_record` (0159's pattern), so they work where the owner has no bypass.
--
-- Expand only (rule 12): a nullable column added under a short lock timeout, new functions, new
-- triggers, a new table. A pod of the previous release writes as before, and its writes are counted.

-- +goose Up
SET LOCAL lock_timeout = '5s';
ALTER TABLE usage_record ADD COLUMN IF NOT EXISTS closing bigint;

-- +goose StatementBegin
-- The people limit's count (PL-04) and the billing count are one definition.
CREATE OR REPLACE FUNCTION count_people() RETURNS bigint
LANGUAGE sql STABLE SET search_path = public, pg_temp AS $$
  SELECT count(*) FROM account
  WHERE kind = 'USER' AND deleted_at IS NULL
    AND (status IN ('ACTIVE', 'RESTRICTED')
         OR (status = 'INVITED' AND (redemption_expires_at IS NULL OR redemption_expires_at > now())))
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Whether one account row counts as a person: the same condition, for a transition table's rows.
CREATE OR REPLACE FUNCTION counts_as_person(
  kind account_kind, status account_status, deleted_at timestamptz, expires timestamptz
) RETURNS integer
LANGUAGE sql STABLE SET search_path = public, pg_temp AS $$
  SELECT CASE WHEN kind = 'USER' AND deleted_at IS NULL
                   AND (status IN ('ACTIVE', 'RESTRICTED')
                        OR (status = 'INVITED' AND (expires IS NULL OR expires > now())))
              THEN 1 ELSE 0 END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- keep_usage adds one statement's delta to the workspace's day row, or starts the day with a fresh
-- count. Runs as the writer, under the writer's own row policy.
CREATE OR REPLACE FUNCTION keep_usage(kept text, delta bigint) RETURNS void
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
DECLARE
  workspace uuid := current_tenant_id();
  today date := (now() AT TIME ZONE 'UTC')::date;
  counted bigint;
BEGIN
  IF delta = 0 OR workspace IS NULL THEN
    RETURN;
  END IF;
  -- Only while the workspace's row stands and its purge has not begun: the purge's own deletes,
  -- and the cascade from the row itself, record nothing.
  IF NOT EXISTS (SELECT 1 FROM tenant WHERE id = workspace AND purge_started_at IS NULL) THEN
    RETURN;
  END IF;
  UPDATE usage_record
     SET closing = closing + delta, value = greatest(value, closing + delta)
   WHERE tenant_id = workspace AND period = today AND metric = kept AND closing IS NOT NULL;
  IF FOUND THEN
    RETURN;
  END IF;
  counted := CASE kept
    WHEN 'items' THEN (SELECT count(*) FROM work_item)
    WHEN 'media_bytes' THEN (SELECT coalesce(sum(byte_size), 0)::bigint FROM media_object WHERE deleted_at IS NULL)
    WHEN 'accounts' THEN count_people()
  END;
  -- The count before this statement is how the day began.
  INSERT INTO usage_record (tenant_id, period, metric, value, closing)
  VALUES (workspace, today, kept, greatest(counted - delta, counted), counted)
  ON CONFLICT (tenant_id, period, metric) DO UPDATE
     SET closing = coalesce(usage_record.closing, 0) + delta,
         value = greatest(usage_record.value, coalesce(usage_record.closing, 0) + delta);
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION keep_items_inserted() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  PERFORM keep_usage('items', (SELECT count(*) FROM inserted));
  RETURN NULL;
END
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION keep_items_deleted() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  PERFORM keep_usage('items', -(SELECT count(*) FROM deleted));
  RETURN NULL;
END
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION keep_media_inserted() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  PERFORM keep_usage('media_bytes',
    (SELECT coalesce(sum(byte_size), 0)::bigint FROM inserted WHERE deleted_at IS NULL));
  RETURN NULL;
END
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION keep_media_deleted() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  PERFORM keep_usage('media_bytes',
    -(SELECT coalesce(sum(byte_size), 0)::bigint FROM deleted WHERE deleted_at IS NULL));
  RETURN NULL;
END
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION keep_media_updated() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  PERFORM keep_usage('media_bytes',
    (SELECT coalesce(sum(byte_size), 0)::bigint FROM inserted WHERE deleted_at IS NULL)
    - (SELECT coalesce(sum(byte_size), 0)::bigint FROM deleted WHERE deleted_at IS NULL));
  RETURN NULL;
END
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION keep_people_inserted() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  PERFORM keep_usage('accounts', (SELECT coalesce(sum(counts_as_person(
    kind, status, deleted_at, redemption_expires_at)), 0)::bigint FROM inserted));
  RETURN NULL;
END
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION keep_people_deleted() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  PERFORM keep_usage('accounts', -(SELECT coalesce(sum(counts_as_person(
    kind, status, deleted_at, redemption_expires_at)), 0)::bigint FROM deleted));
  RETURN NULL;
END
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION keep_people_updated() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  PERFORM keep_usage('accounts',
    (SELECT coalesce(sum(counts_as_person(kind, status, deleted_at, redemption_expires_at)), 0) FROM inserted)
    - (SELECT coalesce(sum(counts_as_person(kind, status, deleted_at, redemption_expires_at)), 0)::bigint FROM deleted));
  RETURN NULL;
END
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS work_item_usage_inserted ON work_item;
CREATE TRIGGER work_item_usage_inserted AFTER INSERT ON work_item
  REFERENCING NEW TABLE AS inserted FOR EACH STATEMENT EXECUTE FUNCTION keep_items_inserted();
DROP TRIGGER IF EXISTS work_item_usage_deleted ON work_item;
CREATE TRIGGER work_item_usage_deleted AFTER DELETE ON work_item
  REFERENCING OLD TABLE AS deleted FOR EACH STATEMENT EXECUTE FUNCTION keep_items_deleted();
DROP TRIGGER IF EXISTS media_object_usage_inserted ON media_object;
CREATE TRIGGER media_object_usage_inserted AFTER INSERT ON media_object
  REFERENCING NEW TABLE AS inserted FOR EACH STATEMENT EXECUTE FUNCTION keep_media_inserted();
DROP TRIGGER IF EXISTS media_object_usage_deleted ON media_object;
CREATE TRIGGER media_object_usage_deleted AFTER DELETE ON media_object
  REFERENCING OLD TABLE AS deleted FOR EACH STATEMENT EXECUTE FUNCTION keep_media_deleted();
DROP TRIGGER IF EXISTS media_object_usage_updated ON media_object;
CREATE TRIGGER media_object_usage_updated AFTER UPDATE ON media_object
  REFERENCING OLD TABLE AS deleted NEW TABLE AS inserted FOR EACH STATEMENT EXECUTE FUNCTION keep_media_updated();
DROP TRIGGER IF EXISTS account_usage_inserted ON account;
CREATE TRIGGER account_usage_inserted AFTER INSERT ON account
  REFERENCING NEW TABLE AS inserted FOR EACH STATEMENT EXECUTE FUNCTION keep_people_inserted();
DROP TRIGGER IF EXISTS account_usage_deleted ON account;
CREATE TRIGGER account_usage_deleted AFTER DELETE ON account
  REFERENCING OLD TABLE AS deleted FOR EACH STATEMENT EXECUTE FUNCTION keep_people_deleted();
DROP TRIGGER IF EXISTS account_usage_updated ON account;
CREATE TRIGGER account_usage_updated AFTER UPDATE ON account
  REFERENCING OLD TABLE AS deleted NEW TABLE AS inserted FOR EACH STATEMENT EXECUTE FUNCTION keep_people_updated();

CREATE TABLE IF NOT EXISTS usage_archive (
  tenant_id uuid   NOT NULL,
  period    date   NOT NULL,
  metric    text   NOT NULL,
  value     bigint NOT NULL,
  closing   bigint,
  PRIMARY KEY (tenant_id, period, metric)
);
GRANT INSERT, SELECT, UPDATE, DELETE ON usage_archive TO hubtask_app;

-- +goose StatementBegin
DO $owner$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'usage_record' AND policyname = 'doors_read') THEN
    CREATE POLICY doors_read ON usage_record FOR SELECT TO CURRENT_USER USING (true);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'usage_record' AND policyname = 'doors_sweep') THEN
    CREATE POLICY doors_sweep ON usage_record FOR DELETE TO CURRENT_USER USING (true);
  END IF;
END $owner$;
-- +goose StatementEnd

-- +goose StatementBegin
-- The control plane's read of every workspace's counts, live and purged, a page of workspaces at a
-- time; for each workspace and metric the last row before the range rides along, so the reader can
-- carry its closing value into the range's first days.
CREATE OR REPLACE FUNCTION admin_usage(
  from_day date, to_day date, want_metric text, want_tenant uuid, after_tenant uuid, page_size integer
)
RETURNS TABLE (tenant_id uuid, period date, metric text, value bigint, closing bigint, purged boolean)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  WITH counts AS (
    SELECT r.tenant_id, r.period, r.metric, r.value, r.closing, false AS purged FROM usage_record r
    UNION ALL
    SELECT a.tenant_id, a.period, a.metric, a.value, a.closing, true FROM usage_archive a
  ),
  wanted AS (
    SELECT * FROM counts c
    WHERE current_tenant_id() IS NULL
      AND c.period <= to_day
      AND (want_metric IS NULL OR c.metric = want_metric)
      AND (want_tenant IS NULL OR c.tenant_id = want_tenant)
      AND (after_tenant IS NULL OR c.tenant_id > after_tenant)
  ),
  page AS (
    SELECT DISTINCT w.tenant_id FROM wanted w ORDER BY w.tenant_id LIMIT page_size
  ),
  before_range AS (
    SELECT DISTINCT ON (w.tenant_id, w.metric) w.*
    FROM wanted w JOIN page p ON p.tenant_id = w.tenant_id
    WHERE w.period < from_day
    ORDER BY w.tenant_id, w.metric, w.period DESC
  )
  SELECT * FROM before_range
  UNION ALL
  SELECT w.* FROM wanted w JOIN page p ON p.tenant_id = w.tenant_id WHERE w.period >= from_day
  ORDER BY 1, 3, 2
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Counts older than the given day go, inside workspaces and in the installation's ledger.
CREATE OR REPLACE FUNCTION sweep_usage(before_day date) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  swept bigint;
  archived bigint;
BEGIN
  IF current_tenant_id() IS NOT NULL THEN
    RETURN 0;
  END IF;
  DELETE FROM usage_record WHERE period < before_day;
  GET DIAGNOSTICS swept = ROW_COUNT;
  DELETE FROM usage_archive WHERE period < before_day;
  GET DIAGNOSTICS archived = ROW_COUNT;
  RETURN swept + archived;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION admin_usage(date, date, text, uuid, uuid, integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_usage(date, date, text, uuid, uuid, integer) TO hubtask_app;
REVOKE ALL ON FUNCTION sweep_usage(date) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION sweep_usage(date) TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
