-- The search's own bookkeeping (M-09, ADR-0034, ADR-0066): which rows were built by a recipe this
-- installation has moved on from - because the configuration changed under it, or because the
-- recipe did.

-- name: CountStaleSearchDocuments :one
-- The rows whose stored recipe differs from what hubtask_search_recipe() answers today, or that
-- were written before it was recorded (NULL). Row level security narrows it to the workspace the
-- transaction is bound to, which is the workspace the operation is asked for.
SELECT count(*)::bigint AS stale
FROM work_item
WHERE search_configuration IS DISTINCT FROM hubtask_search_recipe(content_language);

-- name: RebuildStaleSearchDocuments :execrows
-- One batch: the document and the configuration rewritten for up to `batch` stale rows. Written
-- directly rather than through the trigger - the trigger fires on a change to the three columns
-- the document is built from, and nothing about those changes here - and with SKIP LOCKED so that a
-- person editing one of the rows is not made to wait for the walk, nor the walk for them.
--
-- The batch is a MATERIALIZED CTE and not an `IN (subquery)`, and that is not style: under row
-- level security the planner put the subquery on the inner side of a nested-loop semi join and
-- re-ran it per outer row, and a re-run after the first update no longer saw that row as stale -
-- so the LIMIT walked on and a batch of one rewrote two. Found by the integration test; the CTE
-- is evaluated once, which is what a batch means.
WITH stale AS MATERIALIZED (
  SELECT id FROM work_item
   WHERE search_configuration IS DISTINCT FROM hubtask_search_recipe(content_language)
   ORDER BY id
   LIMIT sqlc.arg('batch')
     FOR UPDATE SKIP LOCKED
)
UPDATE work_item
   SET search_document = hubtask_search_document(work_item.content_language, work_item.title, work_item.notes),
       search_configuration = hubtask_search_recipe(work_item.content_language)
  FROM stale
 WHERE work_item.id = stale.id;
