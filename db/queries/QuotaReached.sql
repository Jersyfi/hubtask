-- A limit reached, marked once per crossing (migration 0181). Every statement is bounded to the
-- transaction's workspace by its policy and names it through current_tenant_id().

-- name: QuotaReachedMarkedAt :one
-- Read before marking: a refusal at a crossing already told writes nothing.
SELECT EXISTS (
  SELECT 1 FROM quota_reached
  WHERE tenant_id = current_tenant_id() AND quota = sqlc.arg(quota) AND period = sqlc.arg(period)
);

-- name: ForgetOtherQuotaPeriods :exec
-- A mark is for one limit value or one day: marking another forgets the rest of the quota's, so a
-- changed limit or a new day is a new crossing.
DELETE FROM quota_reached
WHERE tenant_id = current_tenant_id() AND quota = sqlc.arg(quota) AND period <> sqlc.arg(period);

-- name: MarkQuotaReached :execrows
INSERT INTO quota_reached (tenant_id, quota, period, reached_at)
VALUES (current_tenant_id(), sqlc.arg(quota), sqlc.arg(period), sqlc.arg(reached_at))
ON CONFLICT (tenant_id, quota, period) DO NOTHING;

-- name: QuotaReachedMarked :one
-- Read before the admitted path removes anything: almost every create finds no mark, and a read
-- is all it then costs.
SELECT EXISTS (
  SELECT 1 FROM quota_reached WHERE tenant_id = current_tenant_id() AND quota = sqlc.arg(quota)
);

-- name: ClearQuotaReached :exec
DELETE FROM quota_reached WHERE tenant_id = current_tenant_id() AND quota = sqlc.arg(quota);
