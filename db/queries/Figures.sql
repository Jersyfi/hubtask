-- A collection's figure (UC-WRK-29, domain-model.md §2 "What counts"): one statement per kind,
-- each over a page of collections at once and grouped by collection, so a level of the navigation
-- costs one statement per kind on it rather than one per collection.
--
-- What each collection offers arrives as parameters the structure in force decided, never from a
-- request (rule 9): `offered` and `dated` are comma-separated item types, one string per
-- collection, beside the collection's identifier in the same position.
--
-- **Hidden entries count nowhere.** An entry is hidden when its type is not offered, or when an
-- ancestor's is not (the shared predicate of domain-model.md §2, "Narrowing hides"). The ancestors
-- are the identifiers of the entry's path, which holds the entry itself last; at most two of them.
--
-- No statement names a tenant: row level security binds each to the transaction's, so another
-- workspace's collection counts nothing.

-- name: CountFigureOpen :many
-- OPEN and FIELD_SUM: the open, unarchived rows directly under each collection
-- (`wi_figure_open_idx`), and the sum of one number field over them. A value counts only where
-- its own definition is a live NUMBER (the visible-values rule of Work.sql); `live` says whether the
-- collection has such a field at all, so that no rows sum to 0 and no field sums to null.
WITH wanted AS (
  SELECT w.collection_id, w.field_key,
         w.field_key <> '' AND EXISTS (
           SELECT 1 FROM custom_field_definition cfd
            WHERE cfd.deleted_at IS NULL AND cfd.kind = 'NUMBER' AND cfd.key = w.field_key
              AND 'TASK'::item_type = ANY (cfd.applies_to)
              AND (cfd.collection_id = w.collection_id OR cfd.collection_id IS NULL)
         ) AS live
    FROM (
      SELECT (sqlc.arg('collection_ids')::uuid[])[i] AS collection_id,
             (sqlc.arg('field_keys')::text[])[i] AS field_key
        FROM generate_series(1, cardinality(sqlc.arg('collection_ids')::uuid[])) AS i
    ) w
)
SELECT w.collection_id::uuid AS collection_id,
       w.live::boolean AS live,
       count(s.id)::int AS open,
       count(s.id) FILTER (WHERE s.summable)::int AS summed,
       coalesce(sum(s.amount) FILTER (WHERE s.summable), 0)::text AS sum
  FROM wanted w
  LEFT JOIN LATERAL (
    SELECT wi.id,
           w.live AND jsonb_typeof(wi.custom_fields -> w.field_key) = 'number' AND EXISTS (
             SELECT 1 FROM custom_field_definition cfd
              WHERE cfd.deleted_at IS NULL AND cfd.kind = 'NUMBER'
                AND cfd.id = (wi.custom_field_refs ->> w.field_key)::uuid
                AND (cfd.collection_id = wi.collection_id OR cfd.collection_id IS NULL)
           ) AS summable,
           CASE WHEN jsonb_typeof(wi.custom_fields -> w.field_key) = 'number'
                THEN (wi.custom_fields ->> w.field_key)::numeric END AS amount
      FROM work_item wi
     WHERE wi.collection_id = w.collection_id
       AND wi.parent_id IS NULL AND wi.is_completed = false
       AND wi.archived_at IS NULL AND wi.deleted_at IS NULL
  ) s ON true
 GROUP BY w.collection_id, w.live;

-- name: CountFigureDue :many
-- DUE: open, unarchived, visible rows of a level whose fields carry a due date, due by the end of
-- the reader's day (`wi_due_idx`). A timed date is due when its instant is before the day's end; an
-- all-day date is a calendar date in its own zone (UTC where none is stored), due when that date is
-- on or before the reader's.
SELECT wi.collection_id::uuid AS collection_id, count(*)::int AS due
  FROM (
    SELECT (sqlc.arg('collection_ids')::uuid[])[i] AS collection_id,
           (sqlc.arg('offered')::text[])[i] AS offered,
           (sqlc.arg('dated')::text[])[i] AS dated
      FROM generate_series(1, cardinality(sqlc.arg('collection_ids')::uuid[])) AS i
  ) w
  JOIN work_item wi ON wi.collection_id = w.collection_id
 WHERE wi.deleted_at IS NULL AND wi.archived_at IS NULL AND wi.is_completed = false
   AND wi.due_at IS NOT NULL
   AND wi.type::text = ANY (string_to_array(w.dated, ','))
   AND NOT EXISTS (
     SELECT 1 FROM unnest(string_to_array(btrim(wi.path, '/'), '/')::uuid[]) AS line(id)
       JOIN work_item a ON a.tenant_id = wi.tenant_id AND a.id = line.id
      WHERE NOT (a.type::text = ANY (string_to_array(w.offered, ',')))
   )
   AND CASE WHEN wi.due_date_only
            THEN (wi.due_at AT TIME ZONE coalesce(wi.due_time_zone, 'UTC'))::date
                 <= (sqlc.arg('today')::text)::date
            ELSE wi.due_at < sqlc.arg('day_end')::timestamptz
       END
 GROUP BY wi.collection_id;

-- name: CountFigureDays :many
-- RHYTHM: the reader's dates of this week on which a visible row of the collection was completed
-- (`wi_figure_done_idx`); archived rows included, because putting away what is done must not take
-- a day off the week. Whoever completed, a date counts once.
SELECT wi.collection_id::uuid AS collection_id,
       array_agg(DISTINCT to_char(wi.completed_at AT TIME ZONE sqlc.arg('zone')::text, 'YYYY-MM-DD'))::text[] AS days
  FROM (
    SELECT (sqlc.arg('collection_ids')::uuid[])[i] AS collection_id,
           (sqlc.arg('offered')::text[])[i] AS offered
      FROM generate_series(1, cardinality(sqlc.arg('collection_ids')::uuid[])) AS i
  ) w
  JOIN work_item wi ON wi.collection_id = w.collection_id
 WHERE wi.is_completed = true AND wi.deleted_at IS NULL
   AND wi.completed_at >= sqlc.arg('week_start')::timestamptz
   AND wi.completed_at <= sqlc.arg('now')::timestamptz
   AND NOT EXISTS (
     SELECT 1 FROM unnest(string_to_array(btrim(wi.path, '/'), '/')::uuid[]) AS line(id)
       JOIN work_item a ON a.tenant_id = wi.tenant_id AND a.id = line.id
      WHERE NOT (a.type::text = ANY (string_to_array(w.offered, ',')))
   )
 GROUP BY wi.collection_id;

-- name: CountFigureProgress :many
-- PROGRESS: the visible leaves of each collection - rows with no visible, untrashed child - and how
-- many are completed. Archived leaves count as they stand, as the roll-up counts every child
-- (I-W5): archiving what is done does not lower the share.
SELECT wi.collection_id::uuid AS collection_id,
       count(*)::int AS total,
       count(*) FILTER (WHERE wi.is_completed)::int AS done
  FROM (
    SELECT (sqlc.arg('collection_ids')::uuid[])[i] AS collection_id,
           (sqlc.arg('offered')::text[])[i] AS offered
      FROM generate_series(1, cardinality(sqlc.arg('collection_ids')::uuid[])) AS i
  ) w
  JOIN work_item wi ON wi.collection_id = w.collection_id
 WHERE wi.deleted_at IS NULL
   AND NOT EXISTS (
     SELECT 1 FROM unnest(string_to_array(btrim(wi.path, '/'), '/')::uuid[]) AS line(id)
       JOIN work_item a ON a.tenant_id = wi.tenant_id AND a.id = line.id
      WHERE NOT (a.type::text = ANY (string_to_array(w.offered, ',')))
   )
   AND NOT EXISTS (
     SELECT 1 FROM work_item c
      WHERE c.tenant_id = wi.tenant_id AND c.parent_id = wi.id AND c.deleted_at IS NULL
        AND c.type::text = ANY (string_to_array(w.offered, ','))
   )
 GROUP BY wi.collection_id;
