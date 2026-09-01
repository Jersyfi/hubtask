-- The three big streams partition by month (H-09, multi-tenancy.md §7): activity_entry,
-- outbox_event and rule_run join audit_log and change_log on the pattern the schema already
-- runs twice - a default catch-all, RLS carried per partition, and a SECURITY DEFINER
-- create-and-repair function for the leader (E-09's precedent).
--
-- THE CONVERSION STRATEGY, recorded as the task demands. These tables exist and carry data, so
-- the conversion is attach-the-existing-table-as-first-partition:
--
--   1. The partition-key-shaped unique index is built CONCURRENTLY on the living table - no
--      write blocked, no lock held long.
--   2. A range CHECK (occurred_at < the first day of next month) is added NOT VALID and then
--      VALIDATEd - a SHARE UPDATE EXCLUSIVE scan that blocks no writer. Every row these tables
--      hold is stamped now() at insert, so nothing lies at or past a boundary in the future.
--   3. One atomic swap per table: rename the table to <t>_history, create the partitioned
--      parent under the old name, ATTACH the history as the (MINVALUE .. boundary) partition -
--      the validated CHECK makes the attach a metadata act, no scan - then the default
--      catch-all and the coming months. Writers block for the milliseconds the swap's lock
--      lasts and never fail: before the swap they hit the old table, after it the parent, and
--      the statements are the same.
--
-- Why not a phased copy: nothing references these tables (no foreign key anywhere points at
-- them), their ids are application-minted UUIDv7, and their writers are plain inserts with an
-- explicit timestamp - the attach is exactly as safe and moves no bytes. This lands before
-- H-11 fills two million rows, which is why the validation scans are cheap today ("before they
-- are big").
--
-- ROLLING UPDATE (deployment.md §5): the old binary keeps working against the new schema - its
-- inserts route through the parent, its reads and updates by id scan the partitions and stay
-- correct. The one old statement that dies is the backup import's ON CONFLICT (id) for
-- activity entries (a partitioned table cannot have a unique index on id alone); the new
-- binary's import names the new key. A rolled-back binary can therefore serve requests but not
-- restore activity entries - recorded here and in the pull request, the one caveat on the
-- rollback promise.

-- +goose NO TRANSACTION
-- +goose Up

-- ============ 1. The partition-key indexes, built without blocking ==========

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS activity_entry_part_pkey
  ON activity_entry (tenant_id, occurred_at, id);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS outbox_event_part_pkey
  ON outbox_event (tenant_id, occurred_at, id);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS rule_run_part_pkey
  ON rule_run (tenant_id, started_at, id);

-- The old single-column primary keys cannot survive the attach - a partition may not carry a
-- second PRIMARY KEY beside the parent's - so each is demoted to a plain unique index: built
-- here without blocking, so the id lookups never lose their index for a moment.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS activity_entry_history_id_idx
  ON activity_entry (id);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS outbox_event_history_id_idx
  ON outbox_event (id);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS rule_run_history_id_idx
  ON rule_run (id);

-- ============ 2. The range checks, validated without blocking ===============
-- The boundary is the first day of next month, computed at each step; if a migration were to
-- straddle midnight of a month's last day the attach falls back to a scan, which is slower and
-- still correct.

-- +goose StatementBegin
DO $checks$
DECLARE
  boundary date := (date_trunc('month', now()) + interval '1 month')::date;
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'activity_entry_history_bound') THEN
    EXECUTE format(
      'ALTER TABLE activity_entry ADD CONSTRAINT activity_entry_history_bound CHECK (occurred_at < %L) NOT VALID',
      boundary);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'outbox_event_history_bound') THEN
    EXECUTE format(
      'ALTER TABLE outbox_event ADD CONSTRAINT outbox_event_history_bound CHECK (occurred_at < %L) NOT VALID',
      boundary);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'rule_run_history_bound') THEN
    EXECUTE format(
      'ALTER TABLE rule_run ADD CONSTRAINT rule_run_history_bound CHECK (started_at < %L) NOT VALID',
      boundary);
  END IF;
END $checks$;
-- +goose StatementEnd

-- Guarded, because a rerun after a partially applied file finds the constraints already
-- consumed by their swaps.
-- +goose StatementBegin
DO $validate$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'activity_entry_history_bound') THEN
    ALTER TABLE activity_entry VALIDATE CONSTRAINT activity_entry_history_bound;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'outbox_event_history_bound') THEN
    ALTER TABLE outbox_event VALIDATE CONSTRAINT outbox_event_history_bound;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'rule_run_history_bound') THEN
    ALTER TABLE rule_run VALIDATE CONSTRAINT rule_run_history_bound;
  END IF;
END $validate$;
-- +goose StatementEnd

-- ============ 3. The swaps, one atomic act per table ========================

-- +goose StatementBegin
DO $swap_activity$
DECLARE
  boundary date := (date_trunc('month', now()) + interval '1 month')::date;
BEGIN
  IF to_regclass('public.activity_entry_history') IS NOT NULL THEN
    RETURN;  -- a rerun; the swap already happened
  END IF;

  ALTER TABLE activity_entry RENAME TO activity_entry_history;
  -- The old primary key gives way to the parent's (a partition may not hold two); the plain
  -- unique index built above keeps the id lookups indexed.
  ALTER TABLE activity_entry_history DROP CONSTRAINT activity_entry_pkey;

  CREATE TABLE activity_entry (
    id           uuid NOT NULL,
    tenant_id    uuid NOT NULL CONSTRAINT activity_entry_tenant_id_fkey REFERENCES tenant(id) ON DELETE CASCADE,
    item_id      uuid,
    container_id uuid,
    actor_type   text NOT NULL CONSTRAINT activity_entry_actor_type_check CHECK (actor_type IN ('USER','SERVICE_ACCOUNT','AUTOMATION','AI_AGENT','SYSTEM')),
    actor_id     uuid,
    verb         text NOT NULL,
    change_set   jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at  timestamptz NOT NULL DEFAULT now(),
    correlation_id uuid,
    causation_id   uuid,
    -- The partition key must be part of every unique constraint (the audit_log lesson).
    PRIMARY KEY (tenant_id, occurred_at, id),
    CONSTRAINT activity_entry_item_id_fkey FOREIGN KEY (tenant_id, item_id)
      REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
  ) PARTITION BY RANGE (occurred_at);
  CREATE INDEX activity_entry_page_idx
    ON activity_entry (tenant_id, item_id, occurred_at DESC, id DESC);

  ALTER TABLE activity_entry ENABLE ROW LEVEL SECURITY;
  ALTER TABLE activity_entry FORCE ROW LEVEL SECURITY;
  CREATE POLICY tenant_isolation ON activity_entry
    USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
  GRANT SELECT, INSERT, UPDATE, DELETE ON activity_entry TO hubtask_app;

  -- The attach is a metadata act: the validated CHECK proves the bound, so nothing is scanned.
  EXECUTE format(
    'ALTER TABLE activity_entry ATTACH PARTITION activity_entry_history FOR VALUES FROM (MINVALUE) TO (%L)',
    boundary);
  ALTER TABLE activity_entry_history DROP CONSTRAINT activity_entry_history_bound;

  CREATE TABLE activity_entry_default PARTITION OF activity_entry DEFAULT;
  ALTER TABLE activity_entry_default ENABLE ROW LEVEL SECURITY;
  ALTER TABLE activity_entry_default FORCE ROW LEVEL SECURITY;
  CREATE POLICY tenant_isolation ON activity_entry_default
    USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
  GRANT SELECT, INSERT, UPDATE, DELETE ON activity_entry_default TO hubtask_app;
END $swap_activity$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $swap_outbox$
DECLARE
  boundary date := (date_trunc('month', now()) + interval '1 month')::date;
BEGIN
  IF to_regclass('public.outbox_event_history') IS NOT NULL THEN
    RETURN;
  END IF;

  ALTER TABLE outbox_event RENAME TO outbox_event_history;
  -- The old primary key gives way to the parent's (a partition may not hold two); the plain
  -- unique index built above keeps the id lookups indexed.
  ALTER TABLE outbox_event_history DROP CONSTRAINT outbox_event_pkey;

  CREATE TABLE outbox_event (
    id              uuid NOT NULL,
    tenant_id       uuid NOT NULL,
    event_type      text NOT NULL,
    subject         text,
    payload         jsonb NOT NULL,
    actor_type      text NOT NULL,
    actor_id        uuid,
    correlation_id  uuid,
    causation_id    uuid,
    causation_depth integer NOT NULL DEFAULT 0,
    occurred_at     timestamptz NOT NULL DEFAULT now(),
    dispatched_at   timestamptz,
    attempts        integer NOT NULL DEFAULT 0,
    locked_until    timestamptz,
    replay          boolean NOT NULL DEFAULT false,
    PRIMARY KEY (tenant_id, occurred_at, id)
  ) PARTITION BY RANGE (occurred_at);
  CREATE INDEX outbox_event_pending_idx ON outbox_event (occurred_at) WHERE dispatched_at IS NULL;
  CREATE INDEX outbox_event_poll_idx ON outbox_event (tenant_id, event_type, occurred_at, id)
    WHERE replay = false;

  ALTER TABLE outbox_event ENABLE ROW LEVEL SECURITY;
  ALTER TABLE outbox_event FORCE ROW LEVEL SECURITY;
  CREATE POLICY tenant_isolation ON outbox_event
    USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
  GRANT SELECT, INSERT, UPDATE, DELETE ON outbox_event TO hubtask_app;

  EXECUTE format(
    'ALTER TABLE outbox_event ATTACH PARTITION outbox_event_history FOR VALUES FROM (MINVALUE) TO (%L)',
    boundary);
  ALTER TABLE outbox_event_history DROP CONSTRAINT outbox_event_history_bound;

  CREATE TABLE outbox_event_default PARTITION OF outbox_event DEFAULT;
  ALTER TABLE outbox_event_default ENABLE ROW LEVEL SECURITY;
  ALTER TABLE outbox_event_default FORCE ROW LEVEL SECURITY;
  CREATE POLICY tenant_isolation ON outbox_event_default
    USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
  GRANT SELECT, INSERT, UPDATE, DELETE ON outbox_event_default TO hubtask_app;
END $swap_outbox$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $swap_rule_run$
DECLARE
  boundary date := (date_trunc('month', now()) + interval '1 month')::date;
BEGIN
  IF to_regclass('public.rule_run_history') IS NOT NULL THEN
    RETURN;
  END IF;

  ALTER TABLE rule_run RENAME TO rule_run_history;
  -- The old primary key gives way to the parent's (a partition may not hold two); the plain
  -- unique index built above keeps the id lookups indexed.
  ALTER TABLE rule_run_history DROP CONSTRAINT rule_run_pkey;

  CREATE TABLE rule_run (
    id           uuid NOT NULL,
    tenant_id    uuid NOT NULL CONSTRAINT rule_run_tenant_id_fkey REFERENCES tenant(id) ON DELETE CASCADE,
    rule_id      uuid NOT NULL,
    event_id     uuid,
    trigger      text NOT NULL DEFAULT 'EVENT',
    triggered_by uuid,
    subject_id   uuid,
    status       text NOT NULL CONSTRAINT rule_run_status_check CHECK (status IN ('RUNNING','WAITING','SUCCEEDED','SKIPPED','FAILED','ABORTED_LOOP','THROTTLED')),
    condition_results jsonb NOT NULL DEFAULT '[]'::jsonb,
    action_results    jsonb NOT NULL DEFAULT '[]'::jsonb,
    occasion     text,
    error_code   text,
    started_at   timestamptz NOT NULL DEFAULT now(),
    finished_at  timestamptz,
    causation_depth integer NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, started_at, id),
    CONSTRAINT rule_run_rule_id_fkey
      FOREIGN KEY (tenant_id, rule_id) REFERENCES automation_rule (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT rule_run_trigger_check
      CHECK (trigger IN ('EVENT','SCHEDULE','RELATIVE_DATE','INBOUND_WEBHOOK','MANUAL','JUMBLE_ENTRY'))
  ) PARTITION BY RANGE (started_at);
  CREATE INDEX rule_run_rule_idx ON rule_run (tenant_id, rule_id, started_at DESC);
  CREATE INDEX rule_run_page_idx ON rule_run (tenant_id, trigger, id DESC);

  ALTER TABLE rule_run ENABLE ROW LEVEL SECURITY;
  ALTER TABLE rule_run FORCE ROW LEVEL SECURITY;
  CREATE POLICY tenant_isolation ON rule_run
    USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
  GRANT SELECT, INSERT, UPDATE, DELETE ON rule_run TO hubtask_app;

  EXECUTE format(
    'ALTER TABLE rule_run ATTACH PARTITION rule_run_history FOR VALUES FROM (MINVALUE) TO (%L)',
    boundary);
  ALTER TABLE rule_run_history DROP CONSTRAINT rule_run_history_bound;

  CREATE TABLE rule_run_default PARTITION OF rule_run DEFAULT;
  ALTER TABLE rule_run_default ENABLE ROW LEVEL SECURITY;
  ALTER TABLE rule_run_default FORCE ROW LEVEL SECURITY;
  CREATE POLICY tenant_isolation ON rule_run_default
    USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
  GRANT SELECT, INSERT, UPDATE, DELETE ON rule_run_default TO hubtask_app;
END $swap_rule_run$;
-- +goose StatementEnd

-- ============ 4. The create-and-repair duty, E-09's precedent generalised ===
-- One function for the three streams rather than three copies of the audit one, because their
-- wants are identical: a month's partition, RLS carried, and - unlike the trail - the full
-- grant, since these tables are legitimately updated and swept. The parent is a parameter and
-- is validated against the closed set, so no caller can aim this at another table.

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION ensure_stream_partition(parent text, month date) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  starts date := date_trunc('month', month)::date;
  ends   date := (date_trunc('month', month) + interval '1 month')::date;
  name   text := parent || '_' || to_char(date_trunc('month', month), 'YYYY_MM');
  target regclass;
BEGIN
  IF parent NOT IN ('activity_entry', 'outbox_event', 'rule_run') THEN
    RAISE EXCEPTION 'ensure_stream_partition: % is not a partitioned stream', parent
      USING ERRCODE = 'invalid_parameter_value';
  END IF;

  target := to_regclass(format('public.%I', name));
  IF target IS NULL THEN
    BEGIN
      EXECUTE format('CREATE TABLE %I PARTITION OF %I FOR VALUES FROM (%L) TO (%L)',
        name, parent, starts, ends);
    EXCEPTION WHEN check_violation OR invalid_table_definition THEN
      -- Rows for that month already sit in the default partition; creating the month now would
      -- have to move them. The default is the catch-all, and living with it for one month is
      -- the honest outcome (ensure_audit_partition's reasoning).
      RETURN NULL;
    END;
    target := to_regclass(format('public.%I', name));
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_class WHERE oid = target AND relrowsecurity) THEN
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', name);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_class WHERE oid = target AND relforcerowsecurity) THEN
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', name);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_policy WHERE polrelid = target AND polname = 'tenant_isolation') THEN
    EXECUTE format($policy$
      CREATE POLICY tenant_isolation ON %I
        USING (tenant_id = current_tenant_id())
        WITH CHECK (tenant_id = current_tenant_id())
    $policy$, name);
  END IF;
  IF NOT has_table_privilege('hubtask_app', target, 'INSERT')
     OR NOT has_table_privilege('hubtask_app', target, 'SELECT')
     OR NOT has_table_privilege('hubtask_app', target, 'UPDATE')
     OR NOT has_table_privilege('hubtask_app', target, 'DELETE') THEN
    EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON %I TO hubtask_app', name);
  END IF;

  RETURN name;
END $$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION ensure_stream_partition(text, date) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ensure_stream_partition(text, date) TO hubtask_app;

-- The retention half (H-09): an aged-out month of a partitioned stream is a dropped partition,
-- not a million-row DELETE. SECURITY DEFINER, because the application role holds no DDL - the
-- one narrow act, the purge_tenant_trail discipline. It refuses to drop the default partition
-- and anything whose upper bound is not wholly past the cutoff, counts the rows as the evidence
-- needs them, and answers NULL when there is nothing to drop.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION drop_stream_partition(parent text, cutoff timestamptz) RETURNS TABLE (
  dropped text, rows_removed bigint
)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  candidate record;
  removed   bigint;
BEGIN
  IF parent NOT IN ('activity_entry', 'outbox_event', 'rule_run') THEN
    RAISE EXCEPTION 'drop_stream_partition: % is not a partitioned stream', parent
      USING ERRCODE = 'invalid_parameter_value';
  END IF;

  FOR candidate IN
    SELECT c.relname AS name,
           (regexp_match(pg_get_expr(c.relpartbound, c.oid), 'TO \(''([^'')]+)''\)'))[1] AS upper_bound
    FROM pg_class c
    JOIN pg_inherits i ON i.inhrelid = c.oid
    JOIN pg_class p ON p.oid = i.inhparent
    WHERE p.relname = parent
      AND pg_get_expr(c.relpartbound, c.oid) NOT LIKE 'DEFAULT%'
    ORDER BY c.relname
  LOOP
    IF candidate.upper_bound IS NULL OR candidate.upper_bound::timestamptz > cutoff THEN
      CONTINUE;
    END IF;
    EXECUTE format('SELECT count(*) FROM %I', candidate.name) INTO removed;
    EXECUTE format('ALTER TABLE %I DETACH PARTITION %I', parent, candidate.name);
    EXECUTE format('DROP TABLE %I', candidate.name);
    dropped := candidate.name;
    rows_removed := removed;
    RETURN NEXT;
  END LOOP;
  RETURN;
END $$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION drop_stream_partition(text, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION drop_stream_partition(text, timestamptz) TO hubtask_app;

-- Seed the coming months, the audit pattern: this month is still the history partition's
-- (its bound runs to next month), so only next month needs a table of its own.
SELECT ensure_stream_partition('activity_entry', (date_trunc('month', now()) + interval '1 month')::date);
SELECT ensure_stream_partition('outbox_event', (date_trunc('month', now()) + interval '1 month')::date);
SELECT ensure_stream_partition('rule_run', (date_trunc('month', now()) + interval '1 month')::date);

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
