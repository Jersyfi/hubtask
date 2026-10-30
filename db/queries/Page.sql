-- The knowledge module's pages (migration 0231, M3). The tenant is the transaction's in every
-- statement (ADR-0010).

-- name: InsertPage :exec
INSERT INTO page (
  id, tenant_id, hub_id, parent_id, title, body, icon, order_key, template, content_language,
  created_by, created_at, updated_by, updated_at, version, daily_for
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('hub_id'), sqlc.narg('parent_id'), sqlc.arg('title'),
  sqlc.arg('body'), sqlc.arg('icon'), sqlc.arg('order_key'), sqlc.arg('template'),
  sqlc.narg('content_language'), sqlc.arg('created_by'), sqlc.arg('at'), sqlc.arg('created_by'),
  sqlc.arg('at'), 1, sqlc.narg('daily_for')
);

-- name: FindPage :one
SELECT id, hub_id, parent_id, title, body, icon, order_key, template, content_language,
       owner_id, verified_until, verified_at, verified_by, archived_at, deleted_at,
       created_by, created_at, updated_by, updated_at, version, restricted, daily_for
FROM page
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: FindPageForUpdate :one
SELECT id, hub_id, parent_id, title, body, icon, order_key, template, content_language,
       owner_id, verified_until, verified_at, verified_by, archived_at, deleted_at,
       created_by, created_at, updated_by, updated_at, version, restricted, daily_for
FROM page
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')
FOR UPDATE;

-- name: UpdatePage :execrows
-- Optimistic: zero rows is a page changed meanwhile.
UPDATE page SET
  parent_id = sqlc.narg('parent_id'), title = sqlc.arg('title'), body = sqlc.arg('body'),
  icon = sqlc.arg('icon'), order_key = sqlc.arg('order_key'), template = sqlc.arg('template'),
  content_language = sqlc.narg('content_language'), owner_id = sqlc.narg('owner_id'),
  verified_until = sqlc.narg('verified_until'), verified_at = sqlc.narg('verified_at'),
  verified_by = sqlc.narg('verified_by'), archived_at = sqlc.narg('archived_at'),
  deleted_at = sqlc.narg('deleted_at'), updated_by = sqlc.arg('updated_by'),
  updated_at = sqlc.arg('updated_at'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id') AND version = sqlc.arg('expected_version');

-- name: ListHubPages :many
-- A space's tree without the bodies, at most a bound past the space's limit.
SELECT id, hub_id, parent_id, title, icon, order_key, template, content_language,
       owner_id, verified_until, verified_at, verified_by, archived_at, deleted_at,
       created_by, created_at, updated_by, updated_at, version, restricted, daily_for
FROM page
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id')
  AND deleted_at IS NULL
  AND (sqlc.arg('include_archived')::boolean OR archived_at IS NULL)
  AND (NOT sqlc.arg('templates_only')::boolean OR template)
ORDER BY parent_id NULLS FIRST, order_key, id
LIMIT sqlc.arg('batch');

-- name: ListTrashedHubPages :many
SELECT id, hub_id, parent_id, title, icon, order_key, template, content_language,
       owner_id, verified_until, verified_at, verified_by, archived_at, deleted_at,
       created_by, created_at, updated_by, updated_at, version, restricted, daily_for
FROM page
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id') AND deleted_at IS NOT NULL
ORDER BY deleted_at DESC, id
LIMIT sqlc.arg('batch');

-- name: PageAncestors :many
-- The page's ancestors, nearest first, bounded by the tree's depth.
WITH RECURSIVE up (id, parent_id, title, depth) AS (
  SELECT p.id, p.parent_id, p.title, 0 FROM page p
   WHERE p.tenant_id = current_tenant_id() AND p.id = sqlc.arg('id')
  UNION ALL
  SELECT p.id, p.parent_id, p.title, up.depth + 1 FROM page p
    JOIN up ON p.id = up.parent_id
   WHERE p.tenant_id = current_tenant_id() AND up.depth < 16
)
SELECT up.id, up.title FROM up WHERE up.depth > 0 ORDER BY up.depth;

-- name: PageSubtree :many
-- The page and every page under it, the page first, bodies included, bounded.
WITH RECURSIVE down (id, depth) AS (
  SELECT p.id, 0 FROM page p WHERE p.tenant_id = current_tenant_id() AND p.id = sqlc.arg('id')
  UNION ALL
  SELECT p.id, down.depth + 1 FROM page p
    JOIN down ON p.parent_id = down.id
   WHERE p.tenant_id = current_tenant_id() AND down.depth < 16
     AND (sqlc.arg('include_trashed')::boolean OR p.deleted_at IS NULL)
)
SELECT p.id, p.hub_id, p.parent_id, p.title, p.body, p.icon, p.order_key, p.template, p.content_language,
       p.owner_id, p.verified_until, p.verified_at, p.verified_by, p.archived_at, p.deleted_at,
       p.created_by, p.created_at, p.updated_by, p.updated_at, p.version, p.restricted, p.daily_for
FROM page p JOIN down ON down.id = p.id
WHERE p.tenant_id = current_tenant_id()
ORDER BY down.depth, p.order_key, p.id
LIMIT sqlc.arg('batch');

-- name: LastSiblingOrderKey :one
SELECT coalesce(max(order_key), '')::text FROM page
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id') AND deleted_at IS NULL
  AND parent_id IS NOT DISTINCT FROM sqlc.narg('parent_id');

-- name: CountHubPages :one
SELECT count(*) FROM page WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id');

-- name: TrashPageSubtree :execrows
-- Marks the page and the pages under it trashed at one moment, which is what a restore brings back.
WITH RECURSIVE down (id, depth) AS (
  SELECT p.id, 0 FROM page p WHERE p.tenant_id = current_tenant_id() AND p.id = sqlc.arg('id')
  UNION ALL
  SELECT p.id, down.depth + 1 FROM page p JOIN down ON p.parent_id = down.id
   WHERE p.tenant_id = current_tenant_id() AND p.deleted_at IS NULL AND down.depth < 16
)
UPDATE page AS t SET deleted_at = sqlc.arg('at'), updated_by = sqlc.arg('by'), updated_at = sqlc.arg('at'),
  version = t.version + 1
WHERE t.tenant_id = current_tenant_id() AND t.id IN (SELECT id FROM down) AND t.deleted_at IS NULL;

-- name: RestorePageSubtree :execrows
-- Brings back the page and the pages trashed with it - at the same moment - and not a page under
-- it that was trashed on its own before.
WITH RECURSIVE down (id, depth) AS (
  SELECT p.id, 0 FROM page p WHERE p.tenant_id = current_tenant_id() AND p.id = sqlc.arg('id')
  UNION ALL
  SELECT p.id, down.depth + 1 FROM page p JOIN down ON p.parent_id = down.id
   WHERE p.tenant_id = current_tenant_id() AND p.deleted_at = sqlc.arg('trashed_at')::timestamptz AND down.depth < 16
)
UPDATE page AS t SET deleted_at = NULL, updated_by = sqlc.arg('by'), updated_at = sqlc.arg('at'),
  version = t.version + 1
WHERE t.tenant_id = current_tenant_id() AND t.id IN (SELECT id FROM down)
  AND t.deleted_at = sqlc.arg('trashed_at')::timestamptz;

-- name: SearchPages :many
-- The pages a text finds in the spaces named, ranked; archived and trashed pages stay out.
SELECT p.id, p.hub_id, p.parent_id, p.title, p.icon, p.updated_at,
       ts_rank_cd(p.search_document, q.query)::float8 AS rank,
       ts_headline(hubtask_text_config(p.content_language), p.body, q.query,
         'MaxFragments=1, MaxWords=24, MinWords=8, StartSel=«, StopSel=»')::text AS excerpt
FROM page p,
     LATERAL (SELECT websearch_to_tsquery(hubtask_text_config(p.content_language), sqlc.arg('text')::text)
                  || websearch_to_tsquery('simple', sqlc.arg('text')::text) AS query) q
WHERE p.tenant_id = current_tenant_id() AND p.hub_id = ANY(sqlc.arg('hub_ids')::uuid[])
  AND p.deleted_at IS NULL AND p.archived_at IS NULL
  AND p.search_document @@ q.query
ORDER BY rank DESC, p.updated_at DESC, p.id
LIMIT sqlc.arg('batch');

-- name: PageSummaries :many
-- The pages among the identifiers that are not trashed, for a reference's resolution.
SELECT id, hub_id, title, icon, archived_at, verified_until, restricted
FROM page
WHERE tenant_id = current_tenant_id() AND id = ANY(sqlc.arg('ids')::uuid[]) AND deleted_at IS NULL;

-- name: PageBacklinks :many
-- The texts that point at an object: pages through page_reference, entries and their comments
-- through entry_reference. A trashed page or entry points nowhere.
SELECT 'PAGE'::text AS source_kind, r.page_id AS source_id, p.hub_id AS hub_id,
       '00000000-0000-0000-0000-000000000000'::uuid AS entry_id, r.strength
FROM page_reference r JOIN page p ON p.tenant_id = r.tenant_id AND p.id = r.page_id
WHERE r.tenant_id = current_tenant_id() AND r.target_kind = sqlc.arg('target_kind')
  AND r.target_id = sqlc.arg('target_id') AND p.deleted_at IS NULL
UNION ALL
SELECT r.source_kind, r.source_id, '00000000-0000-0000-0000-000000000000'::uuid AS hub_id,
       r.entry_id, r.strength
FROM entry_reference r JOIN work_item w ON w.tenant_id = r.tenant_id AND w.id = r.entry_id
WHERE r.tenant_id = current_tenant_id() AND r.target_kind = sqlc.arg('target_kind')
  AND r.target_id = sqlc.arg('target_id') AND w.deleted_at IS NULL
LIMIT sqlc.arg('batch');

-- name: LastPageVersion :one
SELECT seq, title, body, author_id, name, restored, created_at FROM page_version
WHERE tenant_id = current_tenant_id() AND page_id = sqlc.arg('page_id')
ORDER BY seq DESC LIMIT 1;

-- name: InsertPageVersion :exec
INSERT INTO page_version (tenant_id, page_id, seq, title, body, author_id, name, restored, created_at)
VALUES (current_tenant_id(), sqlc.arg('page_id'), sqlc.arg('seq'), sqlc.arg('title'), sqlc.arg('body'),
  sqlc.narg('author_id'), sqlc.arg('name'), sqlc.arg('restored'), sqlc.arg('at'));

-- name: FoldPageVersion :exec
UPDATE page_version SET title = sqlc.arg('title'), body = sqlc.arg('body'), created_at = sqlc.arg('at')
WHERE tenant_id = current_tenant_id() AND page_id = sqlc.arg('page_id') AND seq = sqlc.arg('seq');

-- name: NamePageVersion :execrows
UPDATE page_version SET name = sqlc.arg('name')
WHERE tenant_id = current_tenant_id() AND page_id = sqlc.arg('page_id') AND seq = sqlc.arg('seq');

-- name: ListPageVersions :many
-- A page's history newest first, without the bodies.
SELECT seq, title, author_id, name, restored, created_at, octet_length(body)::int AS size FROM page_version
WHERE tenant_id = current_tenant_id() AND page_id = sqlc.arg('page_id')
  AND (sqlc.arg('before')::int = 0 OR seq < sqlc.arg('before')::int)
ORDER BY seq DESC LIMIT sqlc.arg('batch');

-- name: FindPageVersion :one
SELECT seq, title, body, author_id, name, restored, created_at FROM page_version
WHERE tenant_id = current_tenant_id() AND page_id = sqlc.arg('page_id') AND seq = sqlc.arg('seq');

-- name: InsertPageComment :exec
INSERT INTO page_comment (id, tenant_id, page_id, parent_id, author_id, body, anchor, quote, created_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('page_id'), sqlc.narg('parent_id'), sqlc.arg('author_id'),
  sqlc.arg('body'), sqlc.arg('anchor'), sqlc.arg('quote'), sqlc.arg('at'), 1);

-- name: FindPageComment :one
SELECT id, page_id, parent_id, author_id, body, anchor, quote, resolved_at, resolved_by,
       created_at, edited_at, deleted_at, version
FROM page_comment
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: ListPageComments :many
SELECT id, page_id, parent_id, author_id, body, anchor, quote, resolved_at, resolved_by,
       created_at, edited_at, deleted_at, version
FROM page_comment
WHERE tenant_id = current_tenant_id() AND page_id = sqlc.arg('page_id')
ORDER BY created_at, id
LIMIT sqlc.arg('batch');

-- name: UpdatePageComment :execrows
UPDATE page_comment SET body = sqlc.arg('body'), resolved_at = sqlc.narg('resolved_at'),
  resolved_by = sqlc.narg('resolved_by'), edited_at = sqlc.narg('edited_at'),
  deleted_at = sqlc.narg('deleted_at'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id') AND version = sqlc.arg('expected_version');

-- name: PageChainRestrictions :many
-- For each page among the identifiers, every restricted page on its way up (itself included) with
-- the people it names; a restricted page naming nobody answers one row without an account.
WITH RECURSIVE up (page_id, above_id, parent_id, restricted, depth) AS (
  SELECT p.id, p.id, p.parent_id, p.restricted, 0 FROM page p
   WHERE p.tenant_id = current_tenant_id() AND p.id = ANY(sqlc.arg('ids')::uuid[])
  UNION ALL
  SELECT up.page_id, p.id, p.parent_id, p.restricted, up.depth + 1 FROM page p
    JOIN up ON p.id = up.parent_id
   WHERE p.tenant_id = current_tenant_id() AND up.depth < 16
)
SELECT up.page_id, up.above_id, r.account_id
FROM up
LEFT JOIN page_restriction r
  ON r.tenant_id = current_tenant_id() AND r.page_id = up.above_id
WHERE up.restricted
ORDER BY up.page_id, up.above_id, r.account_id;

-- name: PageRestrictedTo :many
-- The people one page's own restriction names.
SELECT account_id FROM page_restriction
WHERE tenant_id = current_tenant_id() AND page_id = sqlc.arg('page_id')
ORDER BY granted_at, account_id;

-- name: ClearPageRestriction :exec
DELETE FROM page_restriction WHERE tenant_id = current_tenant_id() AND page_id = sqlc.arg('page_id');

-- name: InsertPageRestriction :exec
INSERT INTO page_restriction (tenant_id, page_id, account_id, granted_by, granted_at)
SELECT current_tenant_id(), sqlc.arg('page_id'), a, sqlc.arg('granted_by'), sqlc.arg('at')
FROM unnest(sqlc.arg('account_ids')::uuid[]) AS a
ON CONFLICT (tenant_id, page_id, account_id) DO NOTHING;

-- name: SetPageRestricted :execrows
-- The flag alone; the page's version moves, so that an editor holding it reads it again.
UPDATE page SET restricted = sqlc.arg('restricted'), updated_by = sqlc.arg('updated_by'),
  updated_at = sqlc.arg('updated_at'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id') AND version = sqlc.arg('expected_version');

-- name: InsertPageShare :execrows
INSERT INTO page_share (tenant_id, page_id, account_id, granted_by, granted_at)
VALUES (current_tenant_id(), sqlc.arg('page_id'), sqlc.arg('account_id'), sqlc.arg('granted_by'), sqlc.arg('at'))
ON CONFLICT (tenant_id, page_id, account_id) DO NOTHING;

-- name: DeletePageShare :execrows
DELETE FROM page_share
WHERE tenant_id = current_tenant_id() AND page_id = sqlc.arg('page_id') AND account_id = sqlc.arg('account_id');

-- name: PageShares :many
SELECT page_id, account_id, granted_by, granted_at FROM page_share
WHERE tenant_id = current_tenant_id() AND page_id = sqlc.arg('page_id')
ORDER BY granted_at, account_id
LIMIT sqlc.arg('batch');

-- name: IsPageSharedWith :one
SELECT EXISTS (
  SELECT 1 FROM page_share
  WHERE tenant_id = current_tenant_id() AND page_id = sqlc.arg('page_id') AND account_id = sqlc.arg('account_id')
) AS shared;

-- name: PagesSharedWith :many
-- The living pages shared with a person, newest share first.
SELECT p.id, p.hub_id, p.title, p.icon, p.archived_at, p.verified_until, p.restricted
FROM page_share s JOIN page p ON p.tenant_id = s.tenant_id AND p.id = s.page_id
WHERE s.tenant_id = current_tenant_id() AND s.account_id = sqlc.arg('account_id') AND p.deleted_at IS NULL
ORDER BY s.granted_at DESC, p.id
LIMIT sqlc.arg('batch');

-- name: FindPersonalSpace :one
SELECT ps.hub_id FROM personal_space ps
JOIN container c ON c.tenant_id = ps.tenant_id AND c.id = ps.hub_id
WHERE ps.tenant_id = current_tenant_id() AND ps.account_id = sqlc.arg('account_id') AND c.deleted_at IS NULL;

-- name: ClaimPersonalSpace :one
-- The first claim wins; a later one answers the hub already named, or the one it claims when the
-- named hub has gone.
INSERT INTO personal_space (tenant_id, account_id, hub_id, created_at)
VALUES (current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('hub_id'), sqlc.arg('at'))
ON CONFLICT (tenant_id, account_id) DO UPDATE SET hub_id = EXCLUDED.hub_id, created_at = EXCLUDED.created_at
  WHERE NOT EXISTS (
    SELECT 1 FROM container c
    WHERE c.tenant_id = personal_space.tenant_id AND c.id = personal_space.hub_id AND c.deleted_at IS NULL
  )
RETURNING hub_id;

-- name: FindDailyNote :one
SELECT id FROM page
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id') AND daily_for = sqlc.arg('day')
  AND deleted_at IS NULL;
