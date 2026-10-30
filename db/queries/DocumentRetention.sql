-- What the retention pass removes of the knowledge and whiteboard modules (data-retention.md §3):
-- trashed pages and boards past the trash period (TRASH) and page versions thinned out
-- (PAGE_VERSION). The tenant is the transaction's in every statement (ADR-0010).
--
-- A page is removed from the bottom up: one with a page under it - trashed or not, due or not -
-- waits, so the cascade of `page_parent_fkey` never takes a page this pass did not judge and
-- journal. Every delete repeats its read's conditions, so a page restored or given a child in
-- between stays.

-- name: ExpiredTrashedPages :many
-- The trashed pages without a page under them whose period is up, oldest first, each with the
-- accounts whose own words go with it: its creator and the authors of its versions and comments.
SELECT p.id, p.hub_id, p.deleted_at,
       ARRAY(
         SELECT DISTINCT who FROM (
           SELECT p.created_by AS who
           UNION ALL
           SELECT v.author_id FROM page_version v
            WHERE v.tenant_id = p.tenant_id AND v.page_id = p.id AND v.author_id IS NOT NULL
           UNION ALL
           SELECT c.author_id FROM page_comment c
            WHERE c.tenant_id = p.tenant_id AND c.page_id = p.id AND c.author_id IS NOT NULL
         ) AS contributed
       )::uuid[] AS contributors
FROM page p
WHERE p.tenant_id = current_tenant_id()
  AND p.deleted_at IS NOT NULL AND p.deleted_at < sqlc.arg('cutoff')::timestamptz
  AND NOT (p.id = ANY(coalesce(sqlc.narg('passed')::uuid[], '{}')))
  AND NOT EXISTS (SELECT 1 FROM page c WHERE c.tenant_id = p.tenant_id AND c.parent_id = p.id)
ORDER BY p.deleted_at, p.id
LIMIT sqlc.arg('batch')::int
FOR UPDATE OF p SKIP LOCKED;

-- name: DeleteTrashedPages :execrows
-- Takes its versions, comments, references, media rows, articles and live document with it
-- (cascades and migration 0260's trigger).
DELETE FROM page p
WHERE p.tenant_id = current_tenant_id() AND p.id = ANY(sqlc.arg('ids')::uuid[])
  AND p.deleted_at IS NOT NULL AND p.deleted_at < sqlc.arg('cutoff')::timestamptz
  AND NOT EXISTS (SELECT 1 FROM page c WHERE c.tenant_id = p.tenant_id AND c.parent_id = p.id);

-- name: ExpiredTrashedWhiteboards :many
SELECT w.id, w.hub_id, w.deleted_at,
       ARRAY(
         SELECT DISTINCT who FROM (
           SELECT w.created_by AS who
           UNION ALL
           SELECT v.author_id FROM whiteboard_version v
            WHERE v.tenant_id = w.tenant_id AND v.whiteboard_id = w.id AND v.author_id IS NOT NULL
           UNION ALL
           SELECT c.author_id FROM whiteboard_comment c
            WHERE c.tenant_id = w.tenant_id AND c.whiteboard_id = w.id AND c.author_id IS NOT NULL
         ) AS contributed
       )::uuid[] AS contributors
FROM whiteboard w
WHERE w.tenant_id = current_tenant_id()
  AND w.deleted_at IS NOT NULL AND w.deleted_at < sqlc.arg('cutoff')::timestamptz
  AND NOT (w.id = ANY(coalesce(sqlc.narg('passed')::uuid[], '{}')))
ORDER BY w.deleted_at, w.id
LIMIT sqlc.arg('batch')::int
FOR UPDATE OF w SKIP LOCKED;

-- name: DeleteTrashedWhiteboards :execrows
DELETE FROM whiteboard w
WHERE w.tenant_id = current_tenant_id() AND w.id = ANY(sqlc.arg('ids')::uuid[])
  AND w.deleted_at IS NOT NULL AND w.deleted_at < sqlc.arg('cutoff')::timestamptz;

-- name: DeleteThinnedPageVersions :execrows
-- Page versions past the period that are neither named nor the newest of their day (UTC), oldest
-- first; a version in a held hub or by a held or restricted author stays.
DELETE FROM page_version v
WHERE (v.tenant_id, v.page_id, v.seq) IN (
  SELECT d.tenant_id, d.page_id, d.seq FROM page_version d
  JOIN page p ON p.tenant_id = d.tenant_id AND p.id = d.page_id
  WHERE d.tenant_id = current_tenant_id() AND d.name = ''
    AND d.created_at < sqlc.arg('cutoff')::timestamptz
    AND NOT (p.hub_id = ANY(coalesce(sqlc.narg('held_hubs')::uuid[], '{}')))
    AND (d.author_id IS NULL OR NOT (d.author_id = ANY(coalesce(sqlc.narg('held_accounts')::uuid[], '{}'))))
    AND EXISTS (
      SELECT 1 FROM page_version n
       WHERE n.tenant_id = d.tenant_id AND n.page_id = d.page_id AND n.seq > d.seq
         AND (n.created_at AT TIME ZONE 'UTC')::date = (d.created_at AT TIME ZONE 'UTC')::date)
  ORDER BY d.created_at
  LIMIT sqlc.arg('batch')::int
);

-- name: CountThinnedPageVersions :one
SELECT count(*) FROM (
  SELECT 1 FROM page_version d
  JOIN page p ON p.tenant_id = d.tenant_id AND p.id = d.page_id
  WHERE d.tenant_id = current_tenant_id() AND d.name = ''
    AND d.created_at < sqlc.arg('cutoff')::timestamptz
    AND NOT (p.hub_id = ANY(coalesce(sqlc.narg('held_hubs')::uuid[], '{}')))
    AND (d.author_id IS NULL OR NOT (d.author_id = ANY(coalesce(sqlc.narg('held_accounts')::uuid[], '{}'))))
    AND EXISTS (
      SELECT 1 FROM page_version n
       WHERE n.tenant_id = d.tenant_id AND n.page_id = d.page_id AND n.seq > d.seq
         AND (n.created_at AT TIME ZONE 'UTC')::date = (d.created_at AT TIME ZONE 'UTC')::date)
  LIMIT sqlc.arg('ceiling')::int
) AS due;
