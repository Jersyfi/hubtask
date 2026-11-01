-- How far the outbox is behind, across workspaces, for the health report's backlogs (P-11).
--
-- An operator without Prometheus - a Compose installation - reads the deep report after an
-- incident; its outbox figures were never measured. `outbox_event` is behind row level security,
-- so `outbox_backlog()` is a door of `instance_census`'s shape: how many events wait to be
-- dispatched and how long the oldest has waited. Numbers only, never an event or a workspace. The
-- owner reads the table by a standing policy named `TO CURRENT_USER`, as for `mailbox` (0344);
-- `hubtask_app` gains nothing but the function. The partial index on pending events
-- (`outbox_event_pending_idx`) answers both.
--
-- Expand only and safe for a rolling update: a policy and a function are added, nothing else
-- changes; a pod of the previous release never calls it.

-- +goose Up
-- +goose StatementBegin
DO $owner$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'outbox_event' AND policyname = 'doors_read') THEN
    CREATE POLICY doors_read ON outbox_event FOR SELECT TO CURRENT_USER USING (true);
  END IF;
END $owner$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION outbox_backlog()
RETURNS TABLE (pending bigint, oldest_seconds double precision)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT count(*),
         coalesce(extract(epoch FROM now() - min(occurred_at)), 0)::double precision
  FROM outbox_event
  WHERE dispatched_at IS NULL
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION outbox_backlog() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION outbox_backlog() TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
