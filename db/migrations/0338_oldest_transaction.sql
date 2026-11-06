-- How long the oldest open transaction of this database has run, in seconds, the caller's own left
-- out: while it stays open, no row deleted or updated after it began can be vacuumed away, and a
-- backup's or an export's snapshot held for hours is exactly that (alert A-25).
--
-- A function rather than a query over pg_stat_activity in db/queries, because sqlc knows no
-- statistics view. SECURITY INVOKER: the application role sees the transaction start of its own
-- sessions, the ones it can hold open; no query text is read (rule 10). Expand only (rule 12).

-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION oldest_transaction_seconds()
RETURNS double precision
LANGUAGE sql STABLE SET search_path = pg_catalog, pg_temp AS $$
  SELECT coalesce(extract(epoch FROM max(now() - xact_start)), 0)::double precision
  FROM pg_catalog.pg_stat_activity
  WHERE datname = current_database() AND xact_start IS NOT NULL AND pid <> pg_backend_pid();
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION oldest_transaction_seconds() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION oldest_transaction_seconds() TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
