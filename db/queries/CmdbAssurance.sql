-- The CMDB's assurance (migration 0267): affected CIs, campaigns, groups, assets, software aliases.

-- name: InsertCmdbAffected :execrows
INSERT INTO cmdb_affected (id, tenant_id, item_id, ci_id, added_by, added_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('item_id'), sqlc.arg('ci_id'), sqlc.arg('added_by'), sqlc.arg('at'))
ON CONFLICT (tenant_id, item_id, ci_id) DO NOTHING;

-- name: DeleteCmdbAffected :one
DELETE FROM cmdb_affected
WHERE tenant_id = current_tenant_id() AND item_id = sqlc.arg('item_id') AND ci_id = sqlc.arg('ci_id')
RETURNING id, item_id, ci_id, added_by, added_at;

-- name: ListCmdbAffectedOfItem :many
SELECT id, item_id, ci_id, added_by, added_at
FROM cmdb_affected
WHERE tenant_id = current_tenant_id() AND item_id = sqlc.arg('item_id')
ORDER BY added_at, id;

-- name: ListCmdbAffectedOfCis :many
-- The entries naming any of the CIs, the latest first: the neighbours a collision is read from.
SELECT id, item_id, ci_id, added_by, added_at
FROM cmdb_affected
WHERE tenant_id = current_tenant_id() AND ci_id = ANY(sqlc.arg('ci_ids')::uuid[])
ORDER BY added_at DESC, id
LIMIT sqlc.arg('batch');

-- name: InsertCmdbCampaign :exec
INSERT INTO cmdb_campaign (id, tenant_id, hub_id, name, classes, due_by, due_on, review_months, started_by, started_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('hub_id'), sqlc.arg('name'), sqlc.arg('classes')::text[],
  sqlc.narg('due_by'), sqlc.arg('due_on'), sqlc.arg('review_months'), sqlc.arg('started_by'), sqlc.arg('started_at'));

-- name: InsertCmdbAttestations :exec
INSERT INTO cmdb_attestation (tenant_id, campaign_id, ci_id, owner_id)
SELECT current_tenant_id(), sqlc.arg('campaign_id'), c.ci_id, c.owner_id
FROM jsonb_to_recordset(sqlc.arg('asked')::jsonb) AS c(ci_id uuid, owner_id uuid);

-- name: FindCmdbCampaign :one
SELECT id, hub_id, name, classes, due_by, due_on, review_months, started_by, started_at, closed_at, closed_by, version
FROM cmdb_campaign WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: ListCmdbCampaigns :many
SELECT id, hub_id, name, classes, due_by, due_on, review_months, started_by, started_at, closed_at, closed_by, version
FROM cmdb_campaign
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id')
ORDER BY started_at DESC, id
LIMIT sqlc.arg('batch');

-- name: CloseCmdbCampaign :execrows
UPDATE cmdb_campaign SET closed_at = sqlc.arg('at'), closed_by = sqlc.arg('by'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id') AND closed_at IS NULL
  AND version = sqlc.arg('expected_version');

-- name: ListCmdbAttestations :many
SELECT campaign_id, ci_id, owner_id, verdict, attested_by, attested_at
FROM cmdb_attestation
WHERE tenant_id = current_tenant_id() AND campaign_id = sqlc.arg('campaign_id')
ORDER BY ci_id;

-- name: CountCmdbAttestations :many
SELECT campaign_id, verdict, count(*)::int AS n
FROM cmdb_attestation
WHERE tenant_id = current_tenant_id() AND campaign_id = ANY(sqlc.arg('campaign_ids')::uuid[])
GROUP BY campaign_id, verdict;

-- name: FindCmdbAttestation :one
SELECT campaign_id, ci_id, owner_id, verdict, attested_by, attested_at
FROM cmdb_attestation
WHERE tenant_id = current_tenant_id() AND campaign_id = sqlc.arg('campaign_id') AND ci_id = sqlc.arg('ci_id')
FOR UPDATE;

-- name: AnswerCmdbAttestation :exec
UPDATE cmdb_attestation SET verdict = sqlc.arg('verdict'), attested_by = sqlc.arg('by'), attested_at = sqlc.arg('at')
WHERE tenant_id = current_tenant_id() AND campaign_id = sqlc.arg('campaign_id') AND ci_id = sqlc.arg('ci_id');

-- name: UpsertCmdbGroup :execrows
-- Optimistic on a change: zero rows is a group changed meanwhile.
INSERT INTO cmdb_ci_group (id, tenant_id, hub_id, key, name, filter, service_id, updated_by, updated_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('hub_id'), sqlc.arg('key'), sqlc.arg('name'), sqlc.arg('filter'),
  sqlc.narg('service_id'), sqlc.arg('updated_by'), sqlc.arg('at'))
ON CONFLICT (tenant_id, hub_id, key) DO UPDATE SET
  name = excluded.name, filter = excluded.filter, service_id = excluded.service_id,
  updated_by = excluded.updated_by, updated_at = excluded.updated_at, version = cmdb_ci_group.version + 1
WHERE cmdb_ci_group.version = sqlc.arg('expected_version');

-- name: ListCmdbGroups :many
SELECT id, hub_id, key, name, filter, service_id, updated_by, updated_at, version
FROM cmdb_ci_group
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id')
ORDER BY key;

-- name: FindCmdbGroup :one
SELECT id, hub_id, key, name, filter, service_id, updated_by, updated_at, version
FROM cmdb_ci_group WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: UpsertCmdbAsset :execrows
-- Optimistic on a change: zero rows is an asset changed meanwhile.
INSERT INTO cmdb_asset (ci_id, tenant_id, hub_id, asset_tag, cost_minor, currency, purchased_on, supplier,
  warranty_until, contract_id, stockroom_id, assigned_to, request_id, state, disposal, updated_by, updated_at)
VALUES (sqlc.arg('ci_id'), current_tenant_id(), sqlc.arg('hub_id'), sqlc.arg('asset_tag'), sqlc.arg('cost_minor'),
  sqlc.arg('currency'), sqlc.narg('purchased_on'), sqlc.arg('supplier'), sqlc.narg('warranty_until'),
  sqlc.narg('contract_id'), sqlc.narg('stockroom_id'), sqlc.narg('assigned_to'), sqlc.narg('request_id'),
  sqlc.arg('state'), sqlc.narg('disposal'), sqlc.arg('updated_by'), sqlc.arg('at'))
ON CONFLICT (ci_id) DO UPDATE SET
  asset_tag = excluded.asset_tag, cost_minor = excluded.cost_minor, currency = excluded.currency,
  purchased_on = excluded.purchased_on, supplier = excluded.supplier, warranty_until = excluded.warranty_until,
  contract_id = excluded.contract_id, stockroom_id = excluded.stockroom_id, assigned_to = excluded.assigned_to,
  request_id = excluded.request_id, state = excluded.state, disposal = excluded.disposal,
  updated_by = excluded.updated_by, updated_at = excluded.updated_at, version = cmdb_asset.version + 1
WHERE cmdb_asset.tenant_id = current_tenant_id() AND cmdb_asset.version = sqlc.arg('expected_version');

-- name: FindCmdbAsset :one
SELECT ci_id, hub_id, asset_tag, cost_minor, currency, purchased_on, supplier, warranty_until, contract_id,
  stockroom_id, assigned_to, request_id, state, disposal, updated_by, updated_at, version
FROM cmdb_asset WHERE tenant_id = current_tenant_id() AND ci_id = sqlc.arg('ci_id');

-- name: ListCmdbAssets :many
SELECT ci_id, hub_id, asset_tag, cost_minor, currency, purchased_on, supplier, warranty_until, contract_id,
  stockroom_id, assigned_to, request_id, state, disposal, updated_by, updated_at, version
FROM cmdb_asset
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id') AND ci_id > sqlc.arg('after')::uuid
ORDER BY ci_id
LIMIT sqlc.arg('batch');

-- name: UpsertCmdbSoftwareAlias :execrows
-- Optimistic on a change: zero rows is an alias changed meanwhile.
INSERT INTO cmdb_software_alias (id, tenant_id, match_publisher, match_product, publisher, product, updated_by, updated_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('match_publisher'), sqlc.arg('match_product'),
  sqlc.arg('publisher'), sqlc.arg('product'), sqlc.arg('updated_by'), sqlc.arg('at'))
ON CONFLICT (tenant_id, match_publisher, match_product) DO UPDATE SET
  publisher = excluded.publisher, product = excluded.product, updated_by = excluded.updated_by,
  updated_at = excluded.updated_at, version = cmdb_software_alias.version + 1
WHERE cmdb_software_alias.version = sqlc.arg('expected_version');

-- name: ListCmdbSoftwareAliases :many
SELECT id, match_publisher, match_product, publisher, product, updated_by, updated_at, version
FROM cmdb_software_alias WHERE tenant_id = current_tenant_id()
ORDER BY match_publisher, match_product;

-- name: DeleteCmdbSoftwareAlias :execrows
DELETE FROM cmdb_software_alias WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: CountCmdbInstallations :many
-- How many hosts each software CI of a hub is installed on.
SELECT source_ci_id AS ci_id, count(*)::int AS n
FROM cmdb_relation
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id') AND type_key = 'installed_on'
GROUP BY source_ci_id;
