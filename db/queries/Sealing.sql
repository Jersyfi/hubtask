-- The census a rotation ends on (ADR-0045, security.md §8.1): how many stored values still name
-- each master key. Five places hold a sealed value, and a rotation is only finished when none of
-- them names the key that is about to leave the ring.
--
-- Per tenant, like everything else: row level security bounds every branch of the union to the
-- transaction's workspace, and the control plane sums the answers over the tenants it may see.

-- name: CountSealedValuesByKey :many
SELECT sealed.key_id::text AS key_id, count(*)::bigint AS sealed_values
FROM (
  SELECT secret_key_id AS key_id FROM account_mfa
  UNION ALL
  SELECT client_secret_key_id FROM identity_provider
  UNION ALL
  SELECT secret_key_id FROM webhook_subscription WHERE secret_key_id IS NOT NULL
  UNION ALL
  SELECT previous_secret_key_id FROM webhook_subscription WHERE previous_secret_key_id IS NOT NULL
  UNION ALL
  SELECT credential_key_id FROM backup_target WHERE credential_key_id IS NOT NULL
  UNION ALL
  SELECT named #>> '{}'
  FROM automation_rule,
       LATERAL jsonb_path_query(actions, '$.**.secret_header_sealed.key_id') AS named
  WHERE deleted_at IS NULL
) AS sealed
GROUP BY sealed.key_id
ORDER BY sealed.key_id;
