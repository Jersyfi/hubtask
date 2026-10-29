-- The assets and configuration module (migration 0236, M6). The tenant is the transaction's in
-- every statement (ADR-0010).

-- name: ListCmdbClasses :many
SELECT id, key, parent_key, name, icon, abstract, attributes, rules, required, staleness_days,
       archived_at, created_at, updated_at, version
FROM cmdb_class WHERE tenant_id = current_tenant_id() ORDER BY key;

-- name: UpsertCmdbClass :execrows
-- Optimistic on a change: zero rows is a class changed meanwhile.
INSERT INTO cmdb_class (id, tenant_id, key, parent_key, name, icon, abstract, attributes, rules, required,
  staleness_days, archived_at, created_at, updated_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('key'), sqlc.arg('parent_key'), sqlc.arg('name'),
  sqlc.arg('icon'), sqlc.arg('abstract'), sqlc.arg('attributes'), sqlc.arg('rules'), sqlc.arg('required'),
  sqlc.arg('staleness_days'), sqlc.narg('archived_at'), sqlc.arg('at'), sqlc.arg('at'), 1)
ON CONFLICT (tenant_id, key) DO UPDATE SET
  parent_key = excluded.parent_key, name = excluded.name, icon = excluded.icon, abstract = excluded.abstract,
  attributes = excluded.attributes, rules = excluded.rules, required = excluded.required,
  staleness_days = excluded.staleness_days, archived_at = excluded.archived_at,
  updated_at = excluded.updated_at, version = cmdb_class.version + 1
WHERE cmdb_class.version = sqlc.arg('expected_version');

-- name: ListCmdbRelationTypes :many
SELECT id, key, name, reverse_name, impact, pairs, archived_at, created_at, updated_at, version
FROM cmdb_relation_type WHERE tenant_id = current_tenant_id() ORDER BY key;

-- name: UpsertCmdbRelationType :execrows
INSERT INTO cmdb_relation_type (id, tenant_id, key, name, reverse_name, impact, pairs, archived_at,
  created_at, updated_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('key'), sqlc.arg('name'), sqlc.arg('reverse_name'),
  sqlc.arg('impact'), sqlc.arg('pairs'), sqlc.narg('archived_at'), sqlc.arg('at'), sqlc.arg('at'), 1)
ON CONFLICT (tenant_id, key) DO UPDATE SET
  name = excluded.name, reverse_name = excluded.reverse_name, impact = excluded.impact, pairs = excluded.pairs,
  archived_at = excluded.archived_at, updated_at = excluded.updated_at, version = cmdb_relation_type.version + 1
WHERE cmdb_relation_type.version = sqlc.arg('expected_version');

-- name: ListCmdbSources :many
SELECT id, key, kind, rank, classes, may_create, mapping, last_run_at, created_at, updated_at, version
FROM cmdb_source WHERE tenant_id = current_tenant_id() ORDER BY rank, key;

-- name: UpsertCmdbSource :execrows
INSERT INTO cmdb_source (id, tenant_id, key, kind, rank, classes, may_create, mapping, created_at, updated_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('key'), sqlc.arg('kind'), sqlc.arg('rank'),
  sqlc.arg('classes')::text[], sqlc.arg('may_create'), sqlc.narg('mapping'), sqlc.arg('at'), sqlc.arg('at'), 1)
ON CONFLICT (tenant_id, key) DO UPDATE SET
  kind = excluded.kind, rank = excluded.rank, classes = excluded.classes, may_create = excluded.may_create,
  mapping = excluded.mapping, updated_at = excluded.updated_at, version = cmdb_source.version + 1
WHERE cmdb_source.version = sqlc.arg('expected_version');

-- name: TouchCmdbSource :exec
UPDATE cmdb_source SET last_run_at = sqlc.arg('at')
WHERE tenant_id = current_tenant_id() AND key = sqlc.arg('key');

-- name: InsertCmdbCi :exec
INSERT INTO cmdb_ci (id, tenant_id, hub_id, class_key, name, stage, status, attributes, provenance,
  owner_account_id, owner_group_id, location_ci_id, protection, review_due, quantity, last_seen_at, merged_into,
  created_by, created_at, updated_by, updated_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('hub_id'), sqlc.arg('class_key'), sqlc.arg('name'),
  sqlc.arg('stage'), sqlc.arg('status'), sqlc.arg('attributes'), sqlc.arg('provenance'),
  sqlc.narg('owner_account_id'), sqlc.narg('owner_group_id'), sqlc.narg('location_ci_id'), sqlc.arg('protection'),
  sqlc.narg('review_due'), sqlc.arg('quantity'), sqlc.narg('last_seen_at'), sqlc.narg('merged_into'),
  sqlc.arg('created_by'), sqlc.arg('created_at'), sqlc.arg('created_by'), sqlc.arg('created_at'), 1);

-- name: UpdateCmdbCi :execrows
-- Optimistic: zero rows is a CI changed meanwhile.
UPDATE cmdb_ci SET
  class_key = sqlc.arg('class_key'), name = sqlc.arg('name'), stage = sqlc.arg('stage'), status = sqlc.arg('status'),
  attributes = sqlc.arg('attributes'), provenance = sqlc.arg('provenance'),
  owner_account_id = sqlc.narg('owner_account_id'), owner_group_id = sqlc.narg('owner_group_id'),
  location_ci_id = sqlc.narg('location_ci_id'), protection = sqlc.arg('protection'),
  review_due = sqlc.narg('review_due'), quantity = sqlc.arg('quantity'), last_seen_at = sqlc.narg('last_seen_at'),
  merged_into = sqlc.narg('merged_into'), updated_by = sqlc.arg('updated_by'), updated_at = sqlc.arg('updated_at'),
  version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id') AND version = sqlc.arg('expected_version');

-- name: FindCmdbCi :one
SELECT id, hub_id, class_key, name, stage, status, attributes, provenance, owner_account_id, owner_group_id,
       location_ci_id, protection, review_due, quantity, last_seen_at, merged_into,
       created_by, created_at, updated_by, updated_at, version
FROM cmdb_ci WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: FindCmdbCiForUpdate :one
SELECT id, hub_id, class_key, name, stage, status, attributes, provenance, owner_account_id, owner_group_id,
       location_ci_id, protection, review_due, quantity, last_seen_at, merged_into,
       created_by, created_at, updated_by, updated_at, version
FROM cmdb_ci WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')
FOR UPDATE;

-- name: ListCmdbCis :many
-- A hub's CIs in identifier order after a cursor; classes narrow when given.
SELECT id, hub_id, class_key, name, stage, status, attributes, provenance, owner_account_id, owner_group_id,
       location_ci_id, protection, review_due, quantity, last_seen_at, merged_into,
       created_by, created_at, updated_by, updated_at, version
FROM cmdb_ci
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id')
  AND (cardinality(sqlc.arg('classes')::text[]) = 0 OR class_key = ANY(sqlc.arg('classes')::text[]))
  AND id > sqlc.arg('after')::uuid
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: ListCmdbCisByIDs :many
SELECT id, hub_id, class_key, name, stage, status, attributes, provenance, owner_account_id, owner_group_id,
       location_ci_id, protection, review_due, quantity, last_seen_at, merged_into,
       created_by, created_at, updated_by, updated_at, version
FROM cmdb_ci
WHERE tenant_id = current_tenant_id() AND id = ANY(sqlc.arg('ids')::uuid[]);

-- name: CountCmdbCis :one
SELECT count(*)::int FROM cmdb_ci WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id');

-- name: DeleteCmdbCiIdentifiers :exec
DELETE FROM cmdb_ci_identifier
WHERE tenant_id = current_tenant_id() AND ci_id = sqlc.arg('ci_id') AND name NOT LIKE 'source:%';

-- name: PutCmdbCiIdentifiers :exec
-- Writes the CI's identifiers; a source's own key, once set, stays with its CI.
INSERT INTO cmdb_ci_identifier (tenant_id, ci_id, hub_id, name, value)
SELECT current_tenant_id(), sqlc.arg('ci_id'), sqlc.arg('hub_id'),
  unnest(sqlc.arg('names')::text[]), unnest(sqlc.arg('identifier_values')::text[])
ON CONFLICT (tenant_id, ci_id, name) DO UPDATE SET value = excluded.value;

-- name: FindCmdbIdentifiers :many
-- The index rows of a hub's CIs that carry any of the pairs; merged CIs are left out.
SELECT i.ci_id, c.class_key, i.name, i.value
FROM cmdb_ci_identifier i
JOIN cmdb_ci c ON c.tenant_id = i.tenant_id AND c.id = i.ci_id
WHERE i.tenant_id = current_tenant_id() AND i.hub_id = sqlc.arg('hub_id') AND c.merged_into IS NULL
  AND (i.name, i.value) IN (
    SELECT unnest(sqlc.arg('names')::text[]), unnest(sqlc.arg('identifier_values')::text[]))
LIMIT 5000;

-- name: MoveCmdbNativeKeys :exec
-- A merged duplicate's own keys of its sources go to the survivor, so a rerun finds it.
UPDATE cmdb_ci_identifier d SET ci_id = sqlc.arg('survivor_id')
WHERE d.tenant_id = current_tenant_id() AND d.ci_id = sqlc.arg('duplicate_id') AND d.name LIKE 'source:%'
  AND NOT EXISTS (SELECT 1 FROM cmdb_ci_identifier s
                  WHERE s.tenant_id = d.tenant_id AND s.ci_id = sqlc.arg('survivor_id') AND s.name = d.name);

-- name: DropCmdbCiIdentifiersAll :exec
DELETE FROM cmdb_ci_identifier WHERE tenant_id = current_tenant_id() AND ci_id = sqlc.arg('ci_id');

-- name: PutCmdbRelation :one
-- A relation reported again is seen again; inserted says whether it is new.
INSERT INTO cmdb_relation (id, tenant_id, hub_id, type_key, source_ci_id, target_ci_id, data_source,
  first_seen, last_seen, created_by)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('hub_id'), sqlc.arg('type_key'), sqlc.arg('source_ci_id'),
  sqlc.arg('target_ci_id'), sqlc.arg('data_source'), sqlc.arg('at'), sqlc.arg('at'), sqlc.arg('created_by'))
ON CONFLICT (tenant_id, type_key, source_ci_id, target_ci_id) DO UPDATE SET
  last_seen = excluded.last_seen, data_source = excluded.data_source
RETURNING id, (xmax = 0)::boolean AS inserted;

-- name: FindCmdbRelation :one
SELECT id, hub_id, type_key, source_ci_id, target_ci_id, data_source, first_seen, last_seen, created_by
FROM cmdb_relation WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: DeleteCmdbRelation :execrows
DELETE FROM cmdb_relation WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: ListCmdbRelationsOfCi :many
SELECT id, hub_id, type_key, source_ci_id, target_ci_id, data_source, first_seen, last_seen, created_by
FROM cmdb_relation
WHERE tenant_id = current_tenant_id() AND (source_ci_id = sqlc.arg('ci_id') OR target_ci_id = sqlc.arg('ci_id'))
ORDER BY type_key, id
LIMIT 1000;

-- name: ListCmdbRelationsOfHub :many
SELECT id, hub_id, type_key, source_ci_id, target_ci_id, data_source, first_seen, last_seen, created_by
FROM cmdb_relation
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id') AND id > sqlc.arg('after')::uuid
ORDER BY id
LIMIT sqlc.arg('batch');

-- name: MoveCmdbRelations :exec
-- A merged duplicate's relations go to the survivor; one the survivor already has is dropped
-- below, as is one that would join the survivor to itself.
UPDATE cmdb_relation r SET
  source_ci_id = CASE WHEN r.source_ci_id = sqlc.arg('duplicate_id') THEN sqlc.arg('survivor_id') ELSE r.source_ci_id END,
  target_ci_id = CASE WHEN r.target_ci_id = sqlc.arg('duplicate_id') THEN sqlc.arg('survivor_id') ELSE r.target_ci_id END
WHERE r.tenant_id = current_tenant_id()
  AND (r.source_ci_id = sqlc.arg('duplicate_id') OR r.target_ci_id = sqlc.arg('duplicate_id'))
  AND NOT (r.source_ci_id = sqlc.arg('survivor_id') OR r.target_ci_id = sqlc.arg('survivor_id'))
  AND NOT EXISTS (
    SELECT 1 FROM cmdb_relation s
    WHERE s.tenant_id = r.tenant_id AND s.type_key = r.type_key
      AND s.source_ci_id = CASE WHEN r.source_ci_id = sqlc.arg('duplicate_id') THEN sqlc.arg('survivor_id') ELSE r.source_ci_id END
      AND s.target_ci_id = CASE WHEN r.target_ci_id = sqlc.arg('duplicate_id') THEN sqlc.arg('survivor_id') ELSE r.target_ci_id END);

-- name: DropCmdbRelationsOfCi :exec
DELETE FROM cmdb_relation
WHERE tenant_id = current_tenant_id() AND (source_ci_id = sqlc.arg('ci_id') OR target_ci_id = sqlc.arg('ci_id'));

-- name: WalkCmdbImpact :many
-- The CIs an impact reaches from one CI, each at its nearest depth: downstream, those that suffer
-- when it fails; upstream, those whose failure it suffers. Which types carry impact which way is
-- bound by the application (the catalogue's `impact`); the walk never revisits a CI on its path.
WITH RECURSIVE walk(ci_id, depth, path) AS (
  SELECT sqlc.arg('ci_id')::uuid, 0, ARRAY[sqlc.arg('ci_id')::uuid]
  UNION ALL
  SELECT step.next_id, w.depth + 1, w.path || step.next_id
  FROM walk w
  CROSS JOIN LATERAL (
    SELECT r.source_ci_id AS next_id FROM cmdb_relation r
    WHERE r.tenant_id = current_tenant_id() AND sqlc.arg('downstream')::boolean
      AND r.target_ci_id = w.ci_id AND r.type_key = ANY(sqlc.arg('source_suffers')::text[])
    UNION ALL
    SELECT r.target_ci_id FROM cmdb_relation r
    WHERE r.tenant_id = current_tenant_id() AND sqlc.arg('downstream')::boolean
      AND r.source_ci_id = w.ci_id AND r.type_key = ANY(sqlc.arg('target_suffers')::text[])
    UNION ALL
    SELECT r.target_ci_id FROM cmdb_relation r
    WHERE r.tenant_id = current_tenant_id() AND NOT sqlc.arg('downstream')::boolean
      AND r.source_ci_id = w.ci_id AND r.type_key = ANY(sqlc.arg('source_suffers')::text[])
    UNION ALL
    SELECT r.source_ci_id FROM cmdb_relation r
    WHERE r.tenant_id = current_tenant_id() AND NOT sqlc.arg('downstream')::boolean
      AND r.target_ci_id = w.ci_id AND r.type_key = ANY(sqlc.arg('target_suffers')::text[])
  ) step
  WHERE w.depth < sqlc.arg('max_depth')::int AND NOT step.next_id = ANY(w.path)
)
SELECT ci_id, min(depth)::int AS depth FROM walk WHERE depth > 0
GROUP BY ci_id ORDER BY 2, 1
LIMIT 2000;

-- name: InsertCmdbRun :exec
INSERT INTO cmdb_run (id, tenant_id, hub_id, source_key, actor_id, started_at, counts, decisions)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('hub_id'), sqlc.arg('source_key'), sqlc.arg('actor_id'),
  sqlc.arg('started_at'), sqlc.arg('counts'), sqlc.arg('decisions'));

-- name: ListCmdbRuns :many
SELECT id, hub_id, source_key, actor_id, started_at, counts
FROM cmdb_run WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id')
ORDER BY started_at DESC, id DESC
LIMIT sqlc.arg('batch');

-- name: FindCmdbRun :one
SELECT id, hub_id, source_key, actor_id, started_at, counts, decisions
FROM cmdb_run WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: InsertCmdbHold :exec
INSERT INTO cmdb_hold (id, tenant_id, hub_id, run_id, source_key, item, reason, created_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('hub_id'), sqlc.arg('run_id'), sqlc.arg('source_key'),
  sqlc.arg('item'), sqlc.arg('reason'), sqlc.arg('at'));

-- name: ListCmdbHolds :many
SELECT id, hub_id, run_id, source_key, item, reason, created_at, resolved_at, resolved_by
FROM cmdb_hold
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id') AND resolved_at IS NULL
ORDER BY created_at DESC, id
LIMIT sqlc.arg('batch');

-- name: FindCmdbHold :one
SELECT id, hub_id, run_id, source_key, item, reason, created_at, resolved_at, resolved_by
FROM cmdb_hold WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: ResolveCmdbHold :execrows
UPDATE cmdb_hold SET resolved_at = sqlc.arg('at'), resolved_by = sqlc.arg('by')
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id') AND resolved_at IS NULL;

-- name: OpenCmdbDuplicate :execrows
-- Opens a duplicate to merge unless one is open for the same CIs.
INSERT INTO cmdb_duplicate (id, tenant_id, hub_id, ci_ids, rule, created_at)
SELECT sqlc.arg('id'), current_tenant_id(), sqlc.arg('hub_id'), sqlc.arg('ci_ids')::uuid[], sqlc.arg('rule'), sqlc.arg('at')
WHERE NOT EXISTS (
  SELECT 1 FROM cmdb_duplicate d
  WHERE d.tenant_id = current_tenant_id() AND d.resolved_at IS NULL
    AND d.ci_ids @> sqlc.arg('ci_ids')::uuid[] AND d.ci_ids <@ sqlc.arg('ci_ids')::uuid[]);

-- name: ListCmdbDuplicates :many
SELECT id, hub_id, ci_ids, rule, created_at, resolved_at, resolved_by, survivor_id
FROM cmdb_duplicate
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id') AND resolved_at IS NULL
ORDER BY created_at DESC, id
LIMIT sqlc.arg('batch');

-- name: FindCmdbDuplicate :one
SELECT id, hub_id, ci_ids, rule, created_at, resolved_at, resolved_by, survivor_id
FROM cmdb_duplicate WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: ResolveCmdbDuplicate :execrows
UPDATE cmdb_duplicate SET resolved_at = sqlc.arg('at'), resolved_by = sqlc.arg('by'), survivor_id = sqlc.narg('survivor_id')
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id') AND resolved_at IS NULL;

-- name: ListCmdbConnectors :many
SELECT id, hub_id, key, kind, source_key, base_url, settings, allowed_hosts, every_minutes, enabled,
  collection_id, run_as, (credential_enc IS NOT NULL)::boolean AS has_credential, cursor, next_run_at,
  last_run_at, last_run_id, last_outcome, last_error, failures, created_at, updated_at, version
FROM cmdb_connector
WHERE tenant_id = current_tenant_id() AND (sqlc.narg('hub_id')::uuid IS NULL OR hub_id = sqlc.narg('hub_id'))
ORDER BY key;

-- name: FindCmdbConnector :one
SELECT id, hub_id, key, kind, source_key, base_url, settings, allowed_hosts, every_minutes, enabled,
  collection_id, run_as, (credential_enc IS NOT NULL)::boolean AS has_credential, cursor, next_run_at,
  last_run_at, last_run_id, last_outcome, last_error, failures, created_at, updated_at, version
FROM cmdb_connector
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: LockCmdbConnector :one
SELECT id, hub_id, key, kind, source_key, base_url, settings, allowed_hosts, every_minutes, enabled,
  collection_id, run_as, (credential_enc IS NOT NULL)::boolean AS has_credential, cursor, next_run_at,
  last_run_at, last_run_id, last_outcome, last_error, failures, created_at, updated_at, version
FROM cmdb_connector
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')
FOR UPDATE;

-- name: CountCmdbConnectors :one
SELECT count(*) FROM cmdb_connector WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id');

-- name: UpsertCmdbConnector :execrows
INSERT INTO cmdb_connector (id, tenant_id, hub_id, key, kind, source_key, base_url, settings, allowed_hosts,
  every_minutes, enabled, collection_id, run_as, next_run_at, failures, created_at, updated_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('hub_id'), sqlc.arg('key'), sqlc.arg('kind'),
  sqlc.arg('source_key'), sqlc.arg('base_url'), sqlc.arg('settings'), sqlc.arg('allowed_hosts')::text[],
  sqlc.arg('every_minutes'), sqlc.arg('enabled'), sqlc.narg('collection_id'), sqlc.arg('run_as'),
  sqlc.narg('next_run_at'), sqlc.arg('failures'), sqlc.arg('at'), sqlc.arg('at'), 1)
ON CONFLICT (tenant_id, key) DO UPDATE SET
  source_key = excluded.source_key, base_url = excluded.base_url, settings = excluded.settings,
  allowed_hosts = excluded.allowed_hosts, every_minutes = excluded.every_minutes, enabled = excluded.enabled,
  collection_id = excluded.collection_id, run_as = excluded.run_as, next_run_at = excluded.next_run_at,
  failures = excluded.failures, updated_at = excluded.updated_at, version = cmdb_connector.version + 1
WHERE cmdb_connector.version = sqlc.arg('expected_version') AND cmdb_connector.id = excluded.id;

-- name: SetCmdbConnectorCredential :execrows
UPDATE cmdb_connector SET credential_enc = sqlc.arg('credential_enc'), credential_key_id = sqlc.arg('credential_key_id'),
  updated_at = sqlc.arg('at'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: ReadCmdbConnectorCredential :one
SELECT credential_enc, credential_key_id FROM cmdb_connector
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id') AND credential_enc IS NOT NULL;

-- name: RecordCmdbConnectorRun :exec
UPDATE cmdb_connector SET cursor = sqlc.arg('cursor'), next_run_at = sqlc.narg('next_run_at'),
  last_run_at = sqlc.narg('last_run_at'), last_run_id = sqlc.narg('last_run_id'), last_outcome = sqlc.arg('last_outcome'),
  last_error = sqlc.arg('last_error'), failures = sqlc.arg('failures'), enabled = sqlc.arg('enabled')
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: ListSealedCmdbConnectorCredentials :many
SELECT id, credential_enc, credential_key_id FROM cmdb_connector
WHERE tenant_id = current_tenant_id() AND credential_enc IS NOT NULL AND credential_key_id <> sqlc.arg('current_key_id')
ORDER BY id;

-- name: ResealCmdbConnectorCredential :exec
UPDATE cmdb_connector SET credential_enc = sqlc.arg('credential_enc'), credential_key_id = sqlc.arg('credential_key_id')
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: FindCmdbAlert :one
SELECT id, hub_id, connector_id, dedup_key, status, severity, title, ci_id, entry_id, first_at, last_at,
  resolved_at, count
FROM cmdb_alert
WHERE tenant_id = current_tenant_id() AND connector_id = sqlc.arg('connector_id') AND dedup_key = sqlc.arg('dedup_key')
FOR UPDATE;

-- name: PutCmdbAlert :exec
INSERT INTO cmdb_alert (id, tenant_id, hub_id, connector_id, dedup_key, status, severity, title, ci_id, entry_id,
  first_at, last_at, resolved_at, count)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('hub_id'), sqlc.arg('connector_id'), sqlc.arg('dedup_key'),
  sqlc.arg('status'), sqlc.arg('severity'), sqlc.arg('title'), sqlc.narg('ci_id'), sqlc.narg('entry_id'),
  sqlc.arg('first_at'), sqlc.arg('last_at'), sqlc.narg('resolved_at'), sqlc.arg('count'))
ON CONFLICT (tenant_id, connector_id, dedup_key) DO UPDATE SET
  status = excluded.status, severity = excluded.severity, title = excluded.title, ci_id = excluded.ci_id,
  entry_id = excluded.entry_id, last_at = excluded.last_at, resolved_at = excluded.resolved_at, count = excluded.count;

-- name: ListCmdbAlerts :many
SELECT id, hub_id, connector_id, dedup_key, status, severity, title, ci_id, entry_id, first_at, last_at,
  resolved_at, count
FROM cmdb_alert
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id')
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
ORDER BY last_at DESC, id
LIMIT sqlc.arg('max_rows');

-- name: SeeCmdbIntakeDigest :execrows
INSERT INTO cmdb_intake_seen (tenant_id, connector_id, digest, seen_at)
VALUES (current_tenant_id(), sqlc.arg('connector_id'), sqlc.arg('digest'), sqlc.arg('at'))
ON CONFLICT DO NOTHING;

-- name: ForgetCmdbIntakeDigests :exec
DELETE FROM cmdb_intake_seen
WHERE tenant_id = current_tenant_id() AND connector_id = sqlc.arg('connector_id') AND seen_at < sqlc.arg('before');
