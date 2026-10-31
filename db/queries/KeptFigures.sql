-- Kept figures (UC-WRK-29, migration 0292): a collection's count with the basis it was counted on,
-- answered again only while the collection's number of changes (`collection_figure_changes`) is
-- the one it was counted at. Row level security binds every statement to the transaction's
-- workspace.

-- name: KeptFigures :many
-- Each wanted figure with its collection's number of changes now, and the kept count where there is
-- one; whether it still answers is the caller's to judge from the two numbers. Read before the
-- count it may stand in for, so that a write committed in between can only make it older.
WITH wanted AS (
  SELECT (sqlc.arg('collection_ids')::uuid[])[i] AS collection_id,
         (sqlc.arg('kinds')::text[])[i] AS kind,
         (sqlc.arg('basis_keys')::text[])[i] AS basis_key
    FROM generate_series(1, cardinality(sqlc.arg('collection_ids')::uuid[])) AS i
)
SELECT w.collection_id::uuid AS collection_id, w.kind::text AS kind, w.basis_key::text AS basis_key,
       collection_figure_changes(w.collection_id)::bigint AS changes,
       f.changes AS kept_changes, f.counts AS counts, f.counted_at AS counted_at
  FROM wanted w
  LEFT JOIN collection_figure f
    ON f.collection_id = w.collection_id AND f.kind = w.kind AND f.basis_key = w.basis_key;

-- name: KeepFigure :exec
-- Keeps a count, unless one counted at a later number of changes is kept already: two readers that
-- counted around a write never put the older count back.
INSERT INTO collection_figure (
  tenant_id, collection_id, kind, basis_key, basis, changes, counts, counted_at, verified_at
) VALUES (
  current_tenant_id(), sqlc.arg('collection_id'), sqlc.arg('kind'), sqlc.arg('basis_key'),
  sqlc.arg('basis'), sqlc.arg('changes'), sqlc.arg('counts'), sqlc.arg('counted_at'),
  sqlc.arg('counted_at')
)
ON CONFLICT (tenant_id, collection_id, kind, basis_key) DO UPDATE
  SET basis = EXCLUDED.basis, changes = EXCLUDED.changes, counts = EXCLUDED.counts,
      counted_at = EXCLUDED.counted_at, verified_at = EXCLUDED.verified_at
  WHERE collection_figure.changes <= EXCLUDED.changes;

-- name: FoldFigureTouches :execrows
-- Moves the recorded changes into each collection's stamp in one statement: the rows go and the
-- stamp grows by as many in the same commit, so no reader's number of changes moves.
WITH gone AS (
  DELETE FROM collection_figure_touch
   WHERE tenant_id = current_tenant_id()
  RETURNING tenant_id, collection_id
), folded AS (
  SELECT tenant_id, collection_id, count(*) AS n FROM gone GROUP BY tenant_id, collection_id
)
INSERT INTO collection_figure_stamp (tenant_id, collection_id, folded)
SELECT tenant_id, collection_id, n FROM folded
ON CONFLICT (tenant_id, collection_id) DO UPDATE
  SET folded = collection_figure_stamp.folded + EXCLUDED.folded;

-- name: PruneKeptFigures :execrows
-- Drops what no read can be answered from any more: a count from before the collection's last
-- change, one older than `before`, one of a trashed collection.
DELETE FROM collection_figure f
 WHERE f.tenant_id = current_tenant_id()
   AND (f.counted_at < sqlc.arg('before')::timestamptz
        OR f.changes < collection_figure_changes(f.collection_id)
        OR EXISTS (SELECT 1 FROM container c
                    WHERE c.id = f.collection_id AND c.deleted_at IS NOT NULL));

-- name: PruneFigureStamps :execrows
-- A purged collection's stamp: nothing reads it, and its kept figures went with the collection.
-- The workspace's own stamp stays.
DELETE FROM collection_figure_stamp s
 WHERE s.tenant_id = current_tenant_id()
   AND s.collection_id <> current_tenant_id()
   AND NOT EXISTS (SELECT 1 FROM container c WHERE c.id = s.collection_id);

-- name: KeptFiguresToVerify :many
-- The kept counts a read may still be answered from, of the kinds the entries alone decide, the
-- longest unverified first.
SELECT f.collection_id, f.kind, f.basis_key, f.basis, f.changes, f.counts
  FROM collection_figure f
 WHERE f.tenant_id = current_tenant_id()
   AND f.kind = ANY (sqlc.arg('kinds')::text[])
   AND f.changes = collection_figure_changes(f.collection_id)
 ORDER BY f.verified_at, f.collection_id, f.kind, f.basis_key
 LIMIT sqlc.arg('batch');

-- name: VerifyKeptFigure :execrows
-- Writes a recount over the kept count, unless the row moved on meanwhile.
UPDATE collection_figure
   SET counts = sqlc.arg('counts'), verified_at = sqlc.arg('verified_at')
 WHERE tenant_id = current_tenant_id() AND collection_id = sqlc.arg('collection_id')
   AND kind = sqlc.arg('kind') AND basis_key = sqlc.arg('basis_key')
   AND changes = sqlc.arg('changes');
