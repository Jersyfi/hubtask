-- name: ArchiveTenantUsage :execrows
-- At the purge, in its own transaction and the workspace's scope: the counts move to the
-- installation's ledger before the cascade takes them (multi-tenancy.md §4.3).
INSERT INTO usage_archive (tenant_id, period, metric, value, closing)
SELECT tenant_id, period, metric, value, closing FROM usage_record
WHERE tenant_id = current_tenant_id()
ON CONFLICT (tenant_id, period, metric) DO NOTHING;

-- name: SweepUsage :one
-- The scheduler leader's daily sweep (migration 0183): counts older than the day go, inside
-- workspaces and in the installation's ledger.
SELECT sweep_usage(sqlc.arg('before_day')::date)::bigint AS swept;

-- name: CloseArchivedUsage :execrows
-- The purge day closes each count kept as the day's highest at 0, so a read after it carries no
-- count of a workspace that is gone; the day's highest stays what it was - its last closing count,
-- or the day's own row where the purge day had one.
INSERT INTO usage_archive (tenant_id, period, metric, value, closing)
SELECT current_tenant_id(), (now() AT TIME ZONE 'UTC')::date, kept.metric,
       coalesce((SELECT r.closing FROM usage_record r WHERE r.metric = kept.metric AND r.closing IS NOT NULL
                 ORDER BY r.period DESC LIMIT 1), 0), 0
FROM (VALUES ('items'), ('media_bytes'), ('accounts')) AS kept(metric)
ON CONFLICT (tenant_id, period, metric) DO UPDATE SET closing = 0;

-- name: AdminUsage :many
-- The control plane's read of the use (migration 0183): a page of workspaces' counts, live and
-- purged, through the one door that reads across them. The columns are named, since a function's
-- table type is not a table the generator knows.
SELECT tenant_id::uuid AS tenant_id,
       period::date AS period,
       metric::text AS metric,
       value::bigint AS value,
       coalesce(closing, 0)::bigint AS closing,
       (closing IS NOT NULL)::boolean AS has_closing,
       purged::boolean AS purged
FROM admin_usage(
  sqlc.arg('from_day')::date, sqlc.arg('to_day')::date, sqlc.narg('want_metric')::text,
  sqlc.narg('want_tenant')::uuid, sqlc.narg('after_tenant')::uuid, sqlc.arg('page_size')::integer
);
