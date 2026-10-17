-- The operator's count of an offered provider includes a mandate by a workspace's plan
-- (identity.md §18, multi-tenancy.md §4.2).
--
-- A plan that locks a workspace's ways in with providers among them turns the installation's
-- invite-only providers on there, as an installation lock does everywhere. The count decides whether
-- an offered provider may be removed (`DeleteIdentityProvider` removes it at zero), so a provider a
-- plan mandates counts every live workspace on that plan - unless the installation itself locks the
-- ways in, in which case the installation's lock decides and the plan's is under it.
--
-- The count errs on the safe side where it cannot follow the resolver exactly: a plan's locked
-- methods that do not tighten what the installation leaves open (an open `[PASSWORD]` under a plan's
-- locked `[OIDC]`) are ignored on read, yet counted here. A count too high delays a removal; one too
-- low would remove a way in somebody still has.
--
-- Safe for a rolling update: the signature is unchanged, and a server of the previous release that
-- reads the new count reads more than it mandates, which only delays a removal.
-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION count_provider_offers(provider uuid) RETURNS integer
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT count(*)::integer FROM tenant t
  WHERE current_tenant_id() IS NULL
    AND t.deleted_at IS NULL
    AND (
      coalesce(t.settings -> 'offered_providers', '[]'::jsonb) ? provider::text
      OR EXISTS (
        SELECT 1 FROM identity_provider p
        JOIN instance_setting s ON s.key = 'sign_in.methods'
        WHERE p.id = provider
          AND p.tenant_id IS NULL
          AND p.provisioning = 'INVITED_ONLY'
          AND p.enabled
          AND (p.withdraw_at IS NULL OR p.withdraw_at > now())
          AND s.lock_origin <> 'OPEN'
          AND jsonb_typeof(s.value) = 'array'
          AND (s.value ? 'OIDC' OR NOT s.value ?| ARRAY['PASSWORD', 'OIDC'])
      )
      OR EXISTS (
        SELECT 1 FROM identity_provider p
        JOIN plan_setting ps ON ps.plan_id = t.plan_id AND ps.key = 'sign_in.methods'
        WHERE p.id = provider
          AND p.tenant_id IS NULL
          AND p.provisioning = 'INVITED_ONLY'
          AND p.enabled
          AND (p.withdraw_at IS NULL OR p.withdraw_at > now())
          AND ps.locked
          AND jsonb_typeof(ps.value) = 'array'
          AND (ps.value ? 'OIDC' OR NOT ps.value ?| ARRAY['PASSWORD', 'OIDC'])
          AND NOT EXISTS (
            SELECT 1 FROM instance_setting s WHERE s.key = 'sign_in.methods' AND s.lock_origin <> 'OPEN'
          )
      )
    )
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION count_provider_offers(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION count_provider_offers(uuid) TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
