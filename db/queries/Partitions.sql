-- The stream partitions' duty: the leader keeps this month and next existing for the
-- three partitioned streams, and the retention engine drops what has wholly aged out. Both go
-- through the SECURITY DEFINER functions migration 0068 pins down - the application role holds
-- no DDL, and these two narrow acts are the whole exception.

-- name: EnsureStreamPartition :one
SELECT COALESCE(ensure_stream_partition(sqlc.arg('parent')::text, sqlc.arg('month')::date), '')::text AS partition_name;

-- name: DropAgedStreamPartitions :many
-- The casts are for the generator: it cannot see into the function's OUT table.
SELECT d.dropped::text AS dropped, d.rows_removed::bigint AS rows_removed FROM drop_stream_partition(sqlc.arg('parent')::text, sqlc.arg('default_days')::int) AS d;

-- name: DropAgedAuditPartitions :many
-- The trail's retention (migration 0152): whole months of audit_log past the longest audit period,
-- each workspace's count before a month goes, the months a hold keeps, and the default partition's
-- aged rows. The casts are for the generator, as above.
SELECT coalesce(d.part, '')::text AS part, d.part_month::date AS part_month, d.workspace::uuid AS workspace,
       coalesce(d.entries, 0)::bigint AS entries, d.outcome::text AS outcome
FROM drop_audit_partition(sqlc.arg('default_days')::int) AS d;
