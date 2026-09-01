-- The stream partitions' duty (H-09): the leader keeps this month and next existing for the
-- three partitioned streams, and the retention engine drops what has wholly aged out. Both go
-- through the SECURITY DEFINER functions migration 0068 pins down - the application role holds
-- no DDL, and these two narrow acts are the whole exception.

-- name: EnsureStreamPartition :one
SELECT COALESCE(ensure_stream_partition(sqlc.arg('parent')::text, sqlc.arg('month')::date), '')::text AS partition_name;

-- name: DropAgedStreamPartitions :many
-- The casts are for the generator: it cannot see into the function's OUT table.
SELECT d.dropped::text AS dropped, d.rows_removed::bigint AS rows_removed FROM drop_stream_partition(sqlc.arg('parent')::text, sqlc.arg('default_days')::int) AS d;
