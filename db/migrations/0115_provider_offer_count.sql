-- An offered provider's count is counted, not kept (ADR-0077 §1, SC-26, issue #1121).
--
-- 0114 kept the number of workspaces that switched an installation's provider on in a column, moved
-- by each workspace's own switch. Every other write that changes a workspace's switches - a deletion
-- for good, a destructive restore, an import - left it where it was, and the operator read a number
-- that was no longer true (P-11). The number is now counted where it is read: this function answers
-- how many live workspaces name the provider among their own switches. It answers a number and
-- nothing else, so the installation still learns how many and never which (P-01) - and it answers
-- it only outside a workspace: inside one it counts nothing, so a workspace that calls it directly
-- learns no more of the others than the provider statements tell it (zero). The statements keep
-- their own guard as well; this one is the function's, so no future caller can forget it.
--
-- SECURITY DEFINER for `resolve_tenant`'s reason (0063): the tenant table's row level security
-- admits one workspace at a time, and the count is across them. Read-only and STABLE. Like
-- `resolve_tenant` it depends on its owner bypassing row level security - an installation whose owner
-- does not could sign nobody in either.
--
-- Expand, not yet contract: `offered_workspaces` and `move_provider_offer` stay for one release, so
-- that a server of the previous release, still running during a rolling update, keeps working. A
-- later migration drops both, one release after this one (SC-30, issue #1134).
-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION count_provider_offers(provider uuid) RETURNS integer
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT count(*)::integer FROM tenant
  WHERE current_tenant_id() IS NULL
    AND deleted_at IS NULL
    AND coalesce(settings -> 'offered_providers', '[]'::jsonb) ? provider::text
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION count_provider_offers(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION count_provider_offers(uuid) TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
