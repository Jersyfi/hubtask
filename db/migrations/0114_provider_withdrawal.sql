-- An offered provider is withdrawn with a count, a notice and a way back in (ADR-0076, SC-20).
--
-- `offered_workspaces` is how many workspaces have an installation's provider switched on - a
-- number, never names (P-01). It is moved by each workspace's own switch, inside that workspace's
-- transaction, and by nothing else; no job walks tenants. A workspace may not write the
-- installation's row, so the switch moves the count through `move_provider_offer`, which can do
-- exactly that and nothing more.
--
-- `withdraw_at` is when the offer ends. Until then the provider works; from then it is a way in
-- nowhere, read wherever "offered" is resolved - no scheduled job ends it. Clearing it keeps the
-- offer, and because the connected identities are never touched, sign-in through it comes back.
--
-- Both columns belong to installation rows; a workspace's own row keeps zero and NULL.
-- +goose Up
ALTER TABLE identity_provider ADD COLUMN IF NOT EXISTS offered_workspaces integer NOT NULL DEFAULT 0;
ALTER TABLE identity_provider ADD COLUMN IF NOT EXISTS withdraw_at timestamptz;

-- The count as it stands today, once, so the number starts true. A migration may read across
-- workspaces where a job may not: it runs once, as the owner, before anybody reads the number.
UPDATE identity_provider AS offered
SET offered_workspaces = (
  SELECT count(*) FROM tenant AS workspace
  WHERE workspace.deleted_at IS NULL
    AND coalesce(workspace.settings -> 'offered_providers', '[]'::jsonb) ? offered.id::text
)
WHERE offered.tenant_id IS NULL;

-- SECURITY DEFINER for the reason `resolve_tenant` is (0063): writing the installation's row is the
-- owner's right and a workspace's transaction does not hold it. Narrow by construction: it moves one
-- installation row's count by one step, never below zero, and touches no other column and no
-- workspace's row - so a workspace can say "one more" or "one fewer" and learn nothing.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION move_provider_offer(provider uuid, step integer) RETURNS void
LANGUAGE sql SECURITY DEFINER SET search_path = public, pg_temp AS $$
  UPDATE identity_provider
  SET offered_workspaces = greatest(0, offered_workspaces + step)
  WHERE id = provider AND tenant_id IS NULL AND step IN (-1, 1)
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION move_provider_offer(uuid, integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION move_provider_offer(uuid, integer) TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
