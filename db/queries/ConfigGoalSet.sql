-- A template's goal sets (migration 0440, configuration.md §6).

-- name: InsertConfigGoalSet :execrows
-- A key the workspace already marks is no second period: the caller refuses it.
INSERT INTO config_goal_set (tenant_id, key, period_id, created_at)
VALUES (current_tenant_id(), sqlc.arg('key'), sqlc.arg('period_id'), sqlc.arg('created_at'))
ON CONFLICT (tenant_id, key) DO NOTHING;

-- name: ListConfigGoalSets :many
SELECT s.key, s.period_id, p.name, p.start_on, p.end_on
FROM config_goal_set s
JOIN goal_period p ON p.tenant_id = s.tenant_id AND p.id = s.period_id
WHERE s.tenant_id = current_tenant_id()
ORDER BY s.key
LIMIT 200;
