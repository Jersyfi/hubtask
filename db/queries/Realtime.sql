-- Live documents (migration 0230, F-8). The tenant is the transaction's in every statement
-- (ADR-0010).

-- name: OpenRealtimeDocument :exec
-- The head of a document, created on its first update.
INSERT INTO realtime_document (tenant_id, kind, document_id, created_at, updated_at)
VALUES (current_tenant_id(), sqlc.arg('kind'), sqlc.arg('document_id'), sqlc.arg('at'), sqlc.arg('at'))
ON CONFLICT (tenant_id, kind, document_id) DO NOTHING;

-- name: AdvanceRealtimeHead :one
-- Takes the next place; the row lock orders two writers of one document.
UPDATE realtime_document SET head_seq = head_seq + 1, updated_at = sqlc.arg('at')
WHERE tenant_id = current_tenant_id() AND kind = sqlc.arg('kind') AND document_id = sqlc.arg('document_id')
RETURNING head_seq;

-- name: InsertRealtimeUpdate :exec
INSERT INTO realtime_update (tenant_id, kind, document_id, seq, data, author_id, created_at)
VALUES (current_tenant_id(), sqlc.arg('kind'), sqlc.arg('document_id'), sqlc.arg('seq'), sqlc.arg('data'),
  sqlc.narg('author_id'), sqlc.arg('at'));

-- name: RealtimeHead :one
SELECT head_seq, snapshot_seq FROM realtime_document
WHERE tenant_id = current_tenant_id() AND kind = sqlc.arg('kind') AND document_id = sqlc.arg('document_id');

-- name: LatestRealtimeSnapshot :one
SELECT up_to_seq, state, created_at FROM realtime_snapshot
WHERE tenant_id = current_tenant_id() AND kind = sqlc.arg('kind') AND document_id = sqlc.arg('document_id')
ORDER BY up_to_seq DESC
LIMIT 1;

-- name: RealtimeUpdatesSince :many
SELECT seq, data, author_id, created_at FROM realtime_update
WHERE tenant_id = current_tenant_id() AND kind = sqlc.arg('kind') AND document_id = sqlc.arg('document_id')
  AND seq > sqlc.arg('after')
ORDER BY seq
LIMIT sqlc.arg('batch');

-- name: AdvanceRealtimeSnapshot :execrows
-- Moves the head's snapshot mark forward only; zero rows is a snapshot at or past this one stored
-- meanwhile.
UPDATE realtime_document SET snapshot_seq = sqlc.arg('up_to_seq'), updated_at = sqlc.arg('at')
WHERE tenant_id = current_tenant_id() AND kind = sqlc.arg('kind') AND document_id = sqlc.arg('document_id')
  AND snapshot_seq < sqlc.arg('up_to_seq') AND head_seq >= sqlc.arg('up_to_seq');

-- name: InsertRealtimeSnapshot :exec
INSERT INTO realtime_snapshot (tenant_id, kind, document_id, up_to_seq, state, created_at)
VALUES (current_tenant_id(), sqlc.arg('kind'), sqlc.arg('document_id'), sqlc.arg('up_to_seq'), sqlc.arg('state'),
  sqlc.arg('at'));

-- name: SupersedeRealtimeSnapshots :exec
UPDATE realtime_snapshot SET superseded_at = sqlc.arg('at')
WHERE tenant_id = current_tenant_id() AND kind = sqlc.arg('kind') AND document_id = sqlc.arg('document_id')
  AND up_to_seq < sqlc.arg('up_to_seq') AND superseded_at IS NULL;

-- name: CompactRealtimeUpdates :exec
UPDATE realtime_update SET compacted_at = sqlc.arg('at')
WHERE tenant_id = current_tenant_id() AND kind = sqlc.arg('kind') AND document_id = sqlc.arg('document_id')
  AND seq <= sqlc.arg('up_to_seq') AND compacted_at IS NULL;

-- name: EraseRealtimeDocument :exec
-- The log and the snapshots go with the head (ON DELETE CASCADE).
DELETE FROM realtime_document
WHERE tenant_id = current_tenant_id() AND kind = sqlc.arg('kind') AND document_id = sqlc.arg('document_id');

-- name: CountExpiredRealtimeUpdates :one
-- What retention may remove: compacted updates older than the cutoff, counted up to a ceiling.
SELECT count(*)::int FROM (
  SELECT 1 FROM realtime_update
  WHERE tenant_id = current_tenant_id() AND compacted_at < sqlc.arg('cutoff')
  LIMIT sqlc.arg('ceiling')
) expired;

-- name: CountExpiredRealtimeSnapshots :one
SELECT count(*)::int FROM (
  SELECT 1 FROM realtime_snapshot
  WHERE tenant_id = current_tenant_id() AND superseded_at < sqlc.arg('cutoff')
  LIMIT sqlc.arg('ceiling')
) expired;

-- name: DeleteExpiredRealtimeUpdates :execrows
DELETE FROM realtime_update u
WHERE u.tenant_id = current_tenant_id() AND (u.kind, u.document_id, u.seq) IN (
  SELECT e.kind, e.document_id, e.seq FROM realtime_update e
  WHERE e.tenant_id = current_tenant_id() AND e.compacted_at < sqlc.arg('cutoff')
  LIMIT sqlc.arg('batch'));

-- name: DeleteExpiredRealtimeSnapshots :execrows
DELETE FROM realtime_snapshot s
WHERE s.tenant_id = current_tenant_id() AND (s.kind, s.document_id, s.up_to_seq) IN (
  SELECT e.kind, e.document_id, e.up_to_seq FROM realtime_snapshot e
  WHERE e.tenant_id = current_tenant_id() AND e.superseded_at < sqlc.arg('cutoff')
  LIMIT sqlc.arg('batch'));

-- name: NotifyAwareness :exec
-- A cursor or a presence for the document's connections on the other processes (offline-sync.md
-- §13): delivered at COMMIT on `hubtask_awareness`, never stored. The payload is the adapter's
-- bounded framing; PostgreSQL refuses one of 8000 bytes or more.
SELECT pg_notify('hubtask_awareness', sqlc.arg('payload')::text);
