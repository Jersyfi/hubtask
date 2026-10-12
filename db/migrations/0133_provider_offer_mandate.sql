-- An offered provider the installation's lock turns on counts every live workspace (identity.md §18,
-- UC-INS-11 check 4).
--
-- Locking the installation's ways in with providers among them turns each offered provider admitted
-- as *Only people invited here* on in every workspace, whatever the workspace's own switch says. The
-- server reads that where it reads the switch (`EffectivePolicy.ProvidersLockedIn`,
-- `IdentityProvider.MandatedHere`); nothing is written into the workspaces' settings. The count the
-- operator reads, the confirmation *Withdraw now* repeats and the guard that refuses to remove a
-- provider in use all read this function - so it has to know the mandate too, or the operator
-- would read 0 for a provider that is the way into every workspace, and its removal would delete
-- every connection to it.
--
-- The same predicate as the domain's, asked of the stored rows: the row is the installation's
-- (`tenant_id IS NULL`), admits `INVITED_ONLY`, and its offer stands (switched on, before any
-- announced withdrawal); `sign_in.methods` is locked (`lock_origin` not `OPEN`) and its value holds
-- OIDC - or holds neither known way in, which the domain bounds to the product's default, both of
-- them. test/integration/provider_mandate_test.go holds the two to the same table of lock shapes.
--
-- Safe for a rolling update: the signature is unchanged, and a server of the previous release that
-- reads the new count during the update reads more than it mandates, for those minutes only.
-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION count_provider_offers(provider uuid) RETURNS integer
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT count(*)::integer FROM tenant
  WHERE current_tenant_id() IS NULL
    AND deleted_at IS NULL
    AND (
      coalesce(settings -> 'offered_providers', '[]'::jsonb) ? provider::text
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
