-- A collection's and a hub's milestone: the last moment they were left with nothing open
-- (UC-WRK-29 check 4, domain-model.md §2).
--
-- "Nothing open" counts every entry that is neither archived nor trashed, at any level and whether
-- the collection's structure hides its level or not - the roll-up counts a hidden child's
-- completion too (I-W5), and hidden is not inactive.

-- name: LockHubMilestone :exec
-- Taken by a completion before it asks whether nothing is open, for the rest of its transaction.
-- Two completions finishing a hub's last two entries at once would each see the other's entry still
-- open and record nothing; under this lock the second asks after the first committed, and sees it.
-- Per hub, so completions in different hubs never wait for each other.
SELECT pg_advisory_xact_lock(hashtext('milestone:' || sqlc.arg('hub_id')::uuid::text));

-- name: RecordCollectionMilestone :execrows
-- Records, or moves, the collection's milestone when none of its entries is open. Writes nothing
-- while one is.
INSERT INTO container_milestone (tenant_id, container_id, reached_at)
SELECT current_tenant_id(), sqlc.arg('collection_id')::uuid, sqlc.arg('reached_at')::timestamptz
WHERE NOT EXISTS (
  SELECT 1 FROM work_item
  WHERE tenant_id = current_tenant_id() AND collection_id = sqlc.arg('collection_id')::uuid
    AND is_completed = false AND archived_at IS NULL AND deleted_at IS NULL
)
ON CONFLICT (tenant_id, container_id) DO UPDATE SET reached_at = EXCLUDED.reached_at;

-- name: RecordHubMilestone :execrows
-- Records, or moves, the hub's milestone when no collection in it - neither archived nor trashed -
-- holds an open entry. Writes nothing while one does.
INSERT INTO container_milestone (tenant_id, container_id, reached_at)
SELECT current_tenant_id(), sqlc.arg('hub_id')::uuid, sqlc.arg('reached_at')::timestamptz
WHERE NOT EXISTS (
  SELECT 1 FROM work_item w
  JOIN container c ON c.tenant_id = w.tenant_id AND c.id = w.collection_id
  WHERE w.tenant_id = current_tenant_id() AND c.parent_id = sqlc.arg('hub_id')::uuid
    AND c.archived_at IS NULL AND c.deleted_at IS NULL
    AND w.is_completed = false AND w.archived_at IS NULL AND w.deleted_at IS NULL
)
ON CONFLICT (tenant_id, container_id) DO UPDATE SET reached_at = EXCLUDED.reached_at;

-- name: MilestonesOf :many
-- The milestones of one page of containers.
SELECT container_id, reached_at FROM container_milestone
WHERE tenant_id = current_tenant_id() AND container_id = ANY(sqlc.arg('container_ids')::uuid[]);
