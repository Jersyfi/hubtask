-- Posts (migration 0401, work-surface concept E-15): a page with a date. The tenant is the
-- transaction's in every statement (ADR-0010). Who has seen or confirmed a post is personal data:
-- counted for everybody, listed for the post's writers only, and never written elsewhere (rule 10).

-- name: InsertPost :exec
INSERT INTO post (
  tenant_id, page_id, hub_id, template, publish_at, published_at, featured_until, audience,
  group_ids, discussion, notify, confirm_by
) VALUES (
  current_tenant_id(), sqlc.arg('page_id'), sqlc.arg('hub_id'), sqlc.narg('template'),
  sqlc.narg('publish_at'), sqlc.narg('published_at'), sqlc.narg('featured_until'),
  sqlc.arg('audience'), sqlc.arg('group_ids')::uuid[], sqlc.arg('discussion'), sqlc.arg('notify'),
  sqlc.narg('confirm_by')
);

-- name: FindPost :one
SELECT page_id, hub_id, template, publish_at, published_at, featured_until, audience, group_ids,
       discussion, notify, confirm_by
FROM post
WHERE tenant_id = current_tenant_id() AND page_id = sqlc.arg('page_id');

-- name: FindPostForUpdate :one
SELECT page_id, hub_id, template, publish_at, published_at, featured_until, audience, group_ids,
       discussion, notify, confirm_by
FROM post
WHERE tenant_id = current_tenant_id() AND page_id = sqlc.arg('page_id')
FOR UPDATE;

-- name: UpdatePost :execrows
UPDATE post SET
  template = sqlc.narg('template'), publish_at = sqlc.narg('publish_at'),
  published_at = sqlc.narg('published_at'), featured_until = sqlc.narg('featured_until'),
  audience = sqlc.arg('audience'), group_ids = sqlc.arg('group_ids')::uuid[],
  discussion = sqlc.arg('discussion'), notify = sqlc.arg('notify'), confirm_by = sqlc.narg('confirm_by')
WHERE tenant_id = current_tenant_id() AND page_id = sqlc.arg('page_id');

-- name: PostPageIDs :many
-- Which of the pages named are posts: those stay off the page tree and the devices.
SELECT page_id FROM post
WHERE tenant_id = current_tenant_id() AND page_id = ANY(sqlc.arg('ids')::uuid[]);

-- name: ListHubPosts :many
-- A hub's posts that are not trashed, with how many have seen each and whether the account has seen
-- and confirmed it, featured ones first, then newest first. Who sees which is the application's
-- to judge (rule 2); `mine` and `unread` narrow here.
SELECT po.page_id, po.hub_id, po.template, po.publish_at, po.published_at, po.featured_until,
       po.audience, po.group_ids, po.discussion, po.notify, po.confirm_by,
       p.title, p.icon, p.created_by, p.created_at,
       (SELECT count(*) FROM post_read r WHERE r.tenant_id = po.tenant_id AND r.page_id = po.page_id)::int AS seen_count,
       EXISTS (SELECT 1 FROM post_read r WHERE r.tenant_id = po.tenant_id AND r.page_id = po.page_id
                 AND r.account_id = sqlc.arg('account_id')) AS seen,
       EXISTS (SELECT 1 FROM post_confirmation c WHERE c.tenant_id = po.tenant_id AND c.page_id = po.page_id
                 AND c.account_id = sqlc.arg('account_id')) AS confirmed
FROM post po
JOIN page p ON p.tenant_id = po.tenant_id AND p.id = po.page_id
WHERE po.tenant_id = current_tenant_id() AND po.hub_id = sqlc.arg('hub_id') AND p.deleted_at IS NULL
  AND (NOT sqlc.arg('mine')::boolean OR p.created_by = sqlc.arg('account_id'))
  AND (NOT sqlc.arg('unread')::boolean OR NOT EXISTS (
         SELECT 1 FROM post_read r WHERE r.tenant_id = po.tenant_id AND r.page_id = po.page_id
           AND r.account_id = sqlc.arg('account_id')))
  AND (sqlc.narg('page_id')::uuid IS NULL OR po.page_id = sqlc.narg('page_id')::uuid)
ORDER BY (po.published_at IS NOT NULL AND po.featured_until > sqlc.arg('now')::timestamptz) DESC,
         coalesce(po.publish_at, p.created_at) DESC, po.page_id
LIMIT sqlc.arg('batch');

-- name: MarkPostSeen :exec
INSERT INTO post_read (tenant_id, page_id, account_id, read_at)
VALUES (current_tenant_id(), sqlc.arg('page_id'), sqlc.arg('account_id'), sqlc.arg('at'))
ON CONFLICT (tenant_id, page_id, account_id) DO NOTHING;

-- name: ConfirmPost :exec
INSERT INTO post_confirmation (tenant_id, page_id, account_id, confirmed_at)
VALUES (current_tenant_id(), sqlc.arg('page_id'), sqlc.arg('account_id'), sqlc.arg('at'))
ON CONFLICT (tenant_id, page_id, account_id) DO NOTHING;

-- name: PostConfirmedBy :many
-- Who confirmed the post: for its writers alone.
SELECT account_id FROM post_confirmation
WHERE tenant_id = current_tenant_id() AND page_id = sqlc.arg('page_id')
ORDER BY confirmed_at, account_id;

-- name: PeopleReachingHub :many
-- The people - not service accounts - who hold a role in the hub, at a collection or an entry of
-- it, or at the workspace unless the hub is private; the workspace's people when no hub is named.
-- Who of them reads the hub is the application's to judge (rule 2). A disabled or deleted account
-- is left out; bounded.
WITH grants AS (
  SELECT m.account_id, m.group_id, m.scope_type, m.scope_id FROM membership m
  UNION ALL
  SELECT g.account_id, g.group_id, g.scope_type, g.scope_id FROM custom_role_grant g
), reaching AS (
  SELECT g.account_id, g.group_id FROM grants g
  WHERE sqlc.narg('hub_id')::uuid IS NULL
     OR (g.scope_type = 'TENANT' AND EXISTS (
           SELECT 1 FROM container h WHERE h.id = sqlc.narg('hub_id')::uuid AND NOT h.private))
     OR (g.scope_type = 'HUB' AND g.scope_id = sqlc.narg('hub_id')::uuid)
     OR (g.scope_type = 'COLLECTION' AND g.scope_id IN (
           SELECT c.id FROM container c WHERE c.parent_id = sqlc.narg('hub_id')::uuid))
     OR (g.scope_type = 'ITEM' AND g.scope_id IN (
           SELECT w.id FROM work_item w JOIN container c ON c.id = w.collection_id
           WHERE c.parent_id = sqlc.narg('hub_id')::uuid))
)
SELECT a.id
FROM account a
WHERE a.tenant_id = current_tenant_id() AND a.deleted_at IS NULL AND a.status <> 'DISABLED'
  AND a.kind = 'USER'
  AND (a.id IN (SELECT r.account_id FROM reaching r WHERE r.account_id IS NOT NULL)
    OR a.id IN (SELECT gm.account_id FROM account_group_member gm
                JOIN reaching r ON r.group_id = gm.group_id))
ORDER BY a.id
LIMIT sqlc.arg('batch');

-- name: GroupsOfAccounts :many
-- Which of the groups named each of the accounts named belongs to.
SELECT gm.account_id, gm.group_id FROM account_group_member gm
WHERE gm.tenant_id = current_tenant_id() AND gm.account_id = ANY(sqlc.arg('account_ids')::uuid[])
ORDER BY gm.account_id, gm.group_id;

-- name: InsertPostReply :exec
INSERT INTO post_reply (id, tenant_id, page_id, parent_id, author_id, body, created_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('page_id'), sqlc.narg('parent_id'),
        sqlc.arg('author_id'), sqlc.arg('body'), sqlc.arg('at'));

-- name: FindPostReply :one
SELECT id, page_id, parent_id, author_id, body, created_at, deleted_at
FROM post_reply
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: ListPostReplies :many
SELECT id, page_id, parent_id, author_id, body, created_at, deleted_at
FROM post_reply
WHERE tenant_id = current_tenant_id() AND page_id = sqlc.arg('page_id')
ORDER BY created_at, id
LIMIT sqlc.arg('batch');

-- name: TakeBackPostReply :execrows
-- Its words and author go; its place stays, so the answers to it keep their thread.
UPDATE post_reply SET body = '', deleted_at = COALESCE(deleted_at, sqlc.arg('at')), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: PostReplyReactions :many
-- The reactions to the post's replies, counted, and whether the account gave each.
SELECT x.reply_id, x.reaction, count(*)::int AS count, bool_or(x.account_id = sqlc.arg('account_id')) AS mine
FROM post_reaction x
JOIN post_reply r ON r.tenant_id = x.tenant_id AND r.id = x.reply_id
WHERE x.tenant_id = current_tenant_id() AND r.page_id = sqlc.arg('page_id')
GROUP BY x.reply_id, x.reaction
ORDER BY x.reply_id, x.reaction;

-- name: InsertPostReaction :exec
INSERT INTO post_reaction (tenant_id, reply_id, account_id, reaction)
VALUES (current_tenant_id(), sqlc.arg('reply_id'), sqlc.arg('account_id'), sqlc.arg('reaction'))
ON CONFLICT DO NOTHING;

-- name: DeletePostReaction :execrows
DELETE FROM post_reaction
WHERE tenant_id = current_tenant_id() AND reply_id = sqlc.arg('reply_id')
  AND account_id = sqlc.arg('account_id') AND reaction = sqlc.arg('reaction');
