-- What a collection's move to another hub leaves pointing back at the hub it leaves (UC-WRK-39,
-- navigation concept §5.3). The tenant is the transaction's in every statement (ADR-0010).

-- name: MoveKeptReleaseEntries :one
-- The collection's entries held by a release of the hub: they stay in that release.
SELECT count(*)::int FROM release_entry re
JOIN release r ON r.tenant_id = re.tenant_id AND r.id = re.release_id
JOIN work_item w ON w.tenant_id = re.tenant_id AND w.id = re.entry_id
WHERE re.tenant_id = current_tenant_id() AND w.collection_id = sqlc.arg('collection_id')
  AND r.hub_id = sqlc.arg('hub_id');

-- name: MoveKeptSlaClocks :one
-- The collection's running or paused clocks measured by a policy of the hub: they keep it.
SELECT count(*)::int FROM entry_sla s
JOIN work_item w ON w.tenant_id = s.tenant_id AND w.id = s.item_id
JOIN sla_policy p ON p.tenant_id = s.tenant_id AND p.key = s.policy_key
WHERE s.tenant_id = current_tenant_id() AND w.collection_id = sqlc.arg('collection_id')
  AND p.hub_id = sqlc.arg('hub_id') AND s.standing IN ('RUNNING', 'PAUSED');
