-- The change log joins the monthly partition duty (N-09, offline-sync.md §7, data-retention.md §4).
--
-- The change log has been partitioned by month since 0001 - one partition for August 2026 and a
-- default everything since lands in - and nothing has ever created the next month's or dropped an
-- aged one: the stream duty of 0068 knew three streams and not this one. Both functions are
-- redefined with `change_log` in the closed set, under the retention kind `SYNC_LOG` - the offline
-- window, the one period the change log, the operation log and the tombstones share, because a
-- change log emptied before the window elapses would let a device that was offline recreate what
-- was deleted (data-catalog.md), and a tombstone kept shorter would not be there to say so.
--
-- What is already in the default partition stays there: this month's rows landed before the
-- month had a partition of its own, and moving them is a rewrite of the busiest table for a
-- month of retention nobody asked for. From next month on every month has its own partition and
-- falls whole when its every row has aged out for everybody - the function's own cutoff, the
-- window as the floor.

-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION ensure_stream_partition(parent text, month date) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  starts date := date_trunc('month', month)::date;
  ends   date := (date_trunc('month', month) + interval '1 month')::date;
  name   text := parent || '_' || to_char(date_trunc('month', month), 'YYYY_MM');
  target regclass;
BEGIN
  IF parent NOT IN ('activity_entry', 'outbox_event', 'rule_run', 'change_log') THEN
    RAISE EXCEPTION 'ensure_stream_partition: % is not a partitioned stream', parent
      USING ERRCODE = 'invalid_parameter_value';
  END IF;

  target := to_regclass(format('public.%I', name));
  IF target IS NULL THEN
    BEGIN
      EXECUTE format('CREATE TABLE %I PARTITION OF %I FOR VALUES FROM (%L) TO (%L)',
        name, parent, starts, ends);
    EXCEPTION WHEN check_violation OR invalid_table_definition OR invalid_object_definition THEN
      -- The month is already covered - its rows sit in the default partition, or the history
      -- partition's open-ended range holds it (every pre-conversion month does). Creating the
      -- month now would have to move rows; living with the covering partition is the honest
      -- outcome (ensure_audit_partition's reasoning).
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

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION drop_stream_partition(parent text, default_days integer) RETURNS TABLE (
  dropped text, rows_removed bigint
)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  kind      text;
  horizon   integer;
  cutoff    timestamptz;
  candidate record;
  removed   bigint;
BEGIN
  kind := CASE parent
    WHEN 'activity_entry' THEN 'ACTIVITY_ENTRY'
    WHEN 'outbox_event'   THEN 'OUTBOX_EVENT'
    WHEN 'rule_run'       THEN 'RULE_RUN'
    WHEN 'change_log'     THEN 'SYNC_LOG'
    ELSE NULL
  END;
  IF kind IS NULL THEN
    RAISE EXCEPTION 'drop_stream_partition: % is not a partitioned stream', parent
      USING ERRCODE = 'invalid_parameter_value';
  END IF;
  IF default_days <= 0 THEN
    -- A stream with no bound has no aged-out month; nothing may fall.
    RETURN;
  END IF;

  PERFORM set_config('hubtask.retention_scan', 'on', true);
  SELECT GREATEST(
    default_days,
    coalesce((SELECT max(retain_days) FROM retention_policy WHERE data_kind = kind), 0),
    coalesce((SELECT max(retain_days + coalesce(then_after_days, 0) + grace_days)
              FROM retention_rule WHERE data_kind = kind AND enabled), 0)
  ) INTO horizon;
  PERFORM set_config('hubtask.retention_scan', '', true);

  cutoff := now() - make_interval(days => horizon);

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

REVOKE ALL ON FUNCTION drop_stream_partition(text, integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION drop_stream_partition(text, integer) TO hubtask_app;

-- Seed the coming month, 0068's pattern: this month is the default partition's, so only next
-- month needs a table of its own.
SELECT ensure_stream_partition('change_log', (date_trunc('month', now()) + interval '1 month')::date);

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
