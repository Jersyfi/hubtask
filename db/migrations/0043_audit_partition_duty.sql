-- +goose Up
-- The duty `0001_init` wrote down and left to whoever came next.
--
-- Its comment is exact, and it is a measured finding rather than a reading of the manual: "A policy
-- on the parent is NOT inherited when a partition is addressed directly: PostgreSQL applies the
-- policies of the relation named in the query. Measured on PostgreSQL 16 - through audit_log one
-- tenant's row, through audit_log_2026_08 both." The same is true of the grants: `REVOKE UPDATE,
-- DELETE, TRUNCATE ON audit_log` covers the parent, and a partition addressed directly is a table
-- of its own.
--
-- So a partition created later without its own policy and its own revokes is a cross-tenant leak
-- with a date on it, and an audit trail the application role can rewrite - which is the one thing
-- the whole of §3 exists to prevent. `0001_init` applied both to the partitions it created and said
-- that the job creating further ones has to do the same. This is that, as a function rather than as
-- a paragraph somebody has to remember.
--
-- It repairs as well as creates. Every check below is asked of the catalogue rather than assumed
-- from the fact that this function created the table: a partition somebody made by hand, in a
-- migration or at a psql prompt, is brought into line the next time the duty runs. That is also
-- what makes it cheap to call on a schedule - in the steady state it reads the catalogue and writes
-- nothing.
--
-- SECURITY DEFINER, because creating a partition of `audit_log` and revoking on it are the owner's
-- rights and the application role holds neither. The two things that make that safe are here rather
-- than in a comment elsewhere: `search_path` is pinned, so nothing the caller sets decides which
-- `audit_log` this means; and the only parameter is a `date`, from which every identifier is
-- derived - there is no string from a caller anywhere in a statement.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION ensure_audit_partition(month date) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  starts date := date_trunc('month', month)::date;
  ends   date := (date_trunc('month', month) + interval '1 month')::date;
  name   text := 'audit_log_' || to_char(date_trunc('month', month), 'YYYY_MM');
  target regclass;
BEGIN
  target := to_regclass(format('public.%I', name));

  IF target IS NULL THEN
    BEGIN
      EXECUTE format(
        'CREATE TABLE %I PARTITION OF audit_log FOR VALUES FROM (%L) TO (%L)', name, starts, ends);
    EXCEPTION WHEN check_violation OR invalid_table_definition THEN
      -- Entries for this month are already in the default partition, and PostgreSQL will not
      -- split them out. NULL rather than an exception: the caller is a scheduled duty, this is
      -- not a failure of it, and moving rows out of a default partition is an operator's decision
      -- about a table that must not be rewritten casually.
      RETURN NULL;
    END;
    target := to_regclass(format('public.%I', name));
  END IF;

  -- Level 1 of §3, on the partition itself.
  IF NOT EXISTS (SELECT 1 FROM pg_class WHERE oid = target AND relrowsecurity) THEN
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', name);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_class WHERE oid = target AND relforcerowsecurity) THEN
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', name);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_policy WHERE polrelid = target AND polname = 'tenant_isolation'
  ) THEN
    EXECUTE format($policy$
      CREATE POLICY tenant_isolation ON %I
        USING (tenant_id = current_tenant_id())
        WITH CHECK (tenant_id = current_tenant_id())
    $policy$, name);
  END IF;

  -- And the grants: append-only for the application role, on this table as on the parent.
  IF has_table_privilege('hubtask_app', target, 'UPDATE')
     OR has_table_privilege('hubtask_app', target, 'DELETE')
     OR has_table_privilege('hubtask_app', target, 'TRUNCATE') THEN
    EXECUTE format('REVOKE UPDATE, DELETE, TRUNCATE ON %I FROM hubtask_app', name);
  END IF;
  IF NOT has_table_privilege('hubtask_app', target, 'INSERT')
     OR NOT has_table_privilege('hubtask_app', target, 'SELECT') THEN
    EXECUTE format('GRANT SELECT, INSERT ON %I TO hubtask_app', name);
  END IF;

  RETURN name;
END $$;
-- +goose StatementEnd

-- The application role may run it and nobody else may. A SECURITY DEFINER function is only as
-- narrow as the grant on it.
REVOKE ALL ON FUNCTION ensure_audit_partition(date) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ensure_audit_partition(date) TO hubtask_app;

-- The months that already have rows, brought into line by the same code that will keep the next
-- ones in line - so that the duty's first run is not also its first test.
SELECT ensure_audit_partition(date_trunc('month', now())::date);
SELECT ensure_audit_partition((date_trunc('month', now()) + interval '1 month')::date);

-- +goose Down
-- Development only; in production the rule is to fix forwards (versioning-release.md §4). The
-- partitions stay: dropping one would drop the audit entries in it.
DROP FUNCTION IF EXISTS ensure_audit_partition(date);
