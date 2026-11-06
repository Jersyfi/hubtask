-- How the installation's own mail apps fare (alert A-29, multi-tenancy.md §2.1).
--
-- A mailbox connected through the installation's app (`oauth_installation`, the operator's
-- `HUBTASK_MAIL_OAUTH_*` registration) fails when that app's secret runs out or the provider
-- withdraws it - for every workspace at once, and only the operator can fix it. Each workspace's
-- administrators are told of their own mailbox; nobody saw that they all fail together, because
-- nothing may read mailboxes across workspaces.
--
-- `installation_mail_apps()` is a door of `instance_census`'s shape: per provider preset, how many
-- enabled mailboxes use the installation's app and how many of them have failed for over an hour
-- (the grace `Mailbox.Failing` gives). Numbers per preset name - a fixed, operator-side key -
-- never a row, an address or a workspace. The owner reads `mailbox` across workspaces by a standing
-- policy named `TO CURRENT_USER` (0159); `hubtask_app` gains nothing but the function.
--
-- Expand only and safe for a rolling update: a policy and a function are added, nothing else
-- changes; a pod of the previous release never calls it.

-- +goose Up
-- +goose StatementBegin
DO $owner$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'mailbox' AND policyname = 'doors_read') THEN
    CREATE POLICY doors_read ON mailbox FOR SELECT TO CURRENT_USER USING (true);
  END IF;
END $owner$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION installation_mail_apps()
RETURNS TABLE (provider text, connected bigint, failing bigint)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT oauth_provider,
         count(*),
         count(*) FILTER (WHERE failing_since <= now() - interval '1 hour')
  FROM mailbox
  WHERE enabled AND oauth_installation AND oauth_provider IS NOT NULL
  GROUP BY oauth_provider
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION installation_mail_apps() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION installation_mail_apps() TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
