-- The trail's period, kept (audit.md §3, data-retention.md §3, kind AUDIT): a month of `audit_log`
-- goes as one partition once it ended more than the longest audit period any workspace has ago -
-- never row by row, so the trail stays append-only for everybody, the owner's partition drop aside.
-- `drop_stream_partition`'s shape, for the trail.
--
-- Three refusals are the function's own:
--
-- * A month a tenant-wide legal hold touches stays whole, for every workspace in it: a partition
--   holds every workspace's entries, and a hold prevails over a period (data-retention.md §4.1).
--   It is answered as `held`, so the leader can show it.
-- * The default partition cannot be dropped; its rows past the period are counted and answered as
--   `default_aged`. Moving them out is an operator's decision (migration 0043).
-- * Where the function's owner cannot read across workspaces - neither a superuser nor BYPASSRLS, a
--   managed database's owner under FORCE - it drops nothing and answers `unreadable`: a
--   hold it cannot see must not be read as no hold.
--
-- Before a month goes, the function answers each workspace's count in it, so each workspace's own
-- trail can record what left it. Counts and months only - never an entry's content.
--
-- Forward-only and safe for a rolling update: a new function, which a pod of the previous release
-- never calls.

-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION drop_audit_partition(default_days integer) RETURNS TABLE (
  part text, part_month date, workspace uuid, entries bigint, outcome text
)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  horizon   integer;
  cutoff    timestamptz;
  held      uuid[];
  candidate record;
  counted   record;
  is_held   boolean;
  any_rows  boolean;
BEGIN
  IF default_days <= 0 THEN
    RETURN;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_roles WHERE rolname = current_user AND (rolsuper OR rolbypassrls)
  ) THEN
    part := NULL; part_month := NULL; workspace := NULL; entries := NULL; outcome := 'unreadable';
    RETURN NEXT;
    RETURN;
  END IF;

  PERFORM set_config('hubtask.retention_scan', 'on', true);
  SELECT GREATEST(
    default_days,
    coalesce((SELECT max(p.retain_days) FROM retention_policy p WHERE p.data_kind = 'AUDIT'), 0),
    coalesce((SELECT max(r.retain_days + coalesce(r.then_after_days, 0) + r.grace_days)
              FROM retention_rule r WHERE r.data_kind = 'AUDIT' AND r.enabled), 0)
  ) INTO horizon;
  PERFORM set_config('hubtask.retention_scan', '', true);

  cutoff := now() - make_interval(days => horizon);
  SELECT coalesce(array_agg(DISTINCT h.tenant_id), '{}') INTO held
  FROM legal_hold h WHERE h.scope_kind = 'TENANT' AND h.released_at IS NULL;

  FOR candidate IN
    SELECT c.relname AS name,
           (regexp_match(pg_get_expr(c.relpartbound, c.oid), 'FROM \(''([^'')]+)''\)'))[1] AS lower_bound,
           (regexp_match(pg_get_expr(c.relpartbound, c.oid), 'TO \(''([^'')]+)''\)'))[1] AS upper_bound
    FROM pg_class c
    JOIN pg_inherits i ON i.inhrelid = c.oid
    JOIN pg_class p ON p.oid = i.inhparent
    WHERE p.relname = 'audit_log'
      AND pg_get_expr(c.relpartbound, c.oid) NOT LIKE 'DEFAULT%'
    ORDER BY c.relname
  LOOP
    IF candidate.upper_bound IS NULL OR candidate.upper_bound::timestamptz > cutoff THEN
      CONTINUE;
    END IF;

    EXECUTE format('SELECT EXISTS (SELECT 1 FROM %I WHERE tenant_id = ANY($1))', candidate.name)
      INTO is_held USING held;
    IF is_held THEN
      part := candidate.name; part_month := candidate.lower_bound::date;
      workspace := NULL; entries := NULL; outcome := 'held';
      RETURN NEXT;
      CONTINUE;
    END IF;

    any_rows := false;
    FOR counted IN EXECUTE format(
      'SELECT tenant_id AS tenant, count(*) AS n FROM %I GROUP BY tenant_id', candidate.name)
    LOOP
      any_rows := true;
      part := candidate.name; part_month := candidate.lower_bound::date;
      workspace := counted.tenant; entries := counted.n; outcome := 'dropped';
      RETURN NEXT;
    END LOOP;
    IF NOT any_rows THEN
      part := candidate.name; part_month := candidate.lower_bound::date;
      workspace := NULL; entries := 0; outcome := 'dropped';
      RETURN NEXT;
    END IF;

    EXECUTE format('ALTER TABLE audit_log DETACH PARTITION %I', candidate.name);
    EXECUTE format('DROP TABLE %I', candidate.name);
  END LOOP;

  IF to_regclass('public.audit_log_default') IS NOT NULL THEN
    SELECT count(*) INTO entries FROM audit_log_default d WHERE d.occurred_at < cutoff;
    part := 'audit_log_default'; part_month := NULL; workspace := NULL; outcome := 'default_aged';
    RETURN NEXT;
  END IF;
  RETURN;
END $$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION drop_audit_partition(integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION drop_audit_partition(integer) TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
