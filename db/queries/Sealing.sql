-- The census a rotation ends on (ADR-0045, security.md §8.1): how many stored values still name
-- each master key, by the kind of value. Eight places hold a sealed value - the seventh an
-- authenticator's replacement waiting to be confirmed, the eighth an HTTP call's header secret
-- while its job may still run - and a rotation is only finished when none of them names the key
-- that is about to leave the ring.
--
-- Per tenant, like everything else: row level security bounds every branch of the union to the
-- transaction's workspace, and the control plane sums the answers over the tenants it may see.
-- The two stores whose read policy also admits the installation's rows name the workspace
-- explicitly: those rows are visible to every workspace and belong to none, so counting them here
-- would count each once per workspace. The job table has no policy at all (Job.sql), so its branch
-- names the workspace too.

-- name: CountSealedValuesByStoreAndKey :many
SELECT sealed.store::text AS store, sealed.key_id::text AS key_id, count(*)::bigint AS sealed_values
FROM (
  SELECT 'account_mfa' AS store, secret_key_id AS key_id FROM account_mfa
  UNION ALL
  SELECT 'account_mfa', replacement_secret_key_id FROM account_mfa
  WHERE replacement_secret_key_id IS NOT NULL
  UNION ALL
  SELECT 'identity_provider', client_secret_key_id FROM identity_provider
  WHERE tenant_id = current_tenant_id()
  UNION ALL
  SELECT 'webhook_subscription', secret_key_id FROM webhook_subscription
  WHERE secret_key_id IS NOT NULL
  UNION ALL
  SELECT 'webhook_subscription', previous_secret_key_id FROM webhook_subscription
  WHERE previous_secret_key_id IS NOT NULL
  UNION ALL
  SELECT 'backup_target', credential_key_id FROM backup_target WHERE credential_key_id IS NOT NULL
  UNION ALL
  SELECT 'ai_provider', api_key_key_id FROM ai_provider
  WHERE api_key_key_id IS NOT NULL AND tenant_id = current_tenant_id()
  UNION ALL
  SELECT 'automation_rule', named #>> '{}'
  FROM automation_rule,
       LATERAL jsonb_path_query(actions, '$.**.secret_header_sealed.key_id') AS named
  WHERE deleted_at IS NULL
  UNION ALL
  -- Only a call that will still run: nothing moves a finished, dead-lettered or cancelled job back
  -- to PENDING, so its secret is never opened again.
  SELECT 'automation_http_job', payload #>> '{secret_header_sealed,key_id}'
  FROM job
  WHERE tenant_id = current_tenant_id()
    AND kind = 'automation.http'
    AND state IN ('PENDING', 'RUNNING')
    AND payload #>> '{secret_header_sealed,key_id}' IS NOT NULL
) AS sealed
GROUP BY sealed.store, sealed.key_id
ORDER BY sealed.store, sealed.key_id;

-- name: CountInstallationSealedValuesByStoreAndKey :many
-- The installation's own level, read once in the installation's scope: the sign-in providers it
-- offers and the AI models it offers. These are the only stores that hold rows of no workspace -
-- a backup target cannot, its policy admits none.
SELECT sealed.store::text AS store, sealed.key_id::text AS key_id, count(*)::bigint AS sealed_values
FROM (
  SELECT 'identity_provider' AS store, client_secret_key_id AS key_id FROM identity_provider
  WHERE tenant_id IS NULL
  UNION ALL
  SELECT 'ai_provider', api_key_key_id FROM ai_provider
  WHERE tenant_id IS NULL AND api_key_key_id IS NOT NULL
) AS sealed
GROUP BY sealed.store, sealed.key_id
ORDER BY sealed.store, sealed.key_id;
