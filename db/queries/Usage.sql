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
