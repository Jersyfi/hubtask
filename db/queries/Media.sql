-- The media objects and what references them (C-06, domain-model.md §3.5).
--
-- The tenant is never a parameter: it comes from the transaction's own context through
-- current_tenant_id(), the same value row level security compares against (ADR-0010).

-- name: InsertMediaObject :exec
INSERT INTO media_object (
  id, tenant_id, storage_key, file_name, mime_type, byte_size, usage, status,
  ref_count, created_by, created_at
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('storage_key'), sqlc.narg('file_name'),
  sqlc.arg('mime_type'), sqlc.arg('byte_size'), sqlc.arg('usage'), sqlc.arg('status'),
  0, sqlc.arg('created_by'), sqlc.arg('created_at')
);

-- name: FindMediaObject :one
SELECT id, tenant_id, storage_key, file_name, mime_type, byte_size, checksum, usage, status,
       ref_count, created_by, created_at, deleted_at
FROM media_object
WHERE id = $1;

-- name: SealMediaObject :execrows
-- The confirmation's write: PENDING becomes READY, carrying the judged type and the measured
-- size. Matches only a live PENDING row, so a double confirmation and a confirmation racing the
-- reconciliation job both come back as zero rows - the caller reads the row again and answers
-- from what it finds.
UPDATE media_object SET
  status    = 'READY',
  mime_type = sqlc.arg('mime_type'),
  byte_size = sqlc.arg('byte_size')
WHERE id = sqlc.arg('id')::uuid AND status = 'PENDING' AND deleted_at IS NULL;

-- name: AdjustMediaRefCount :execrows
-- The fast half of reference counting: the use case that adds or drops a reference moves the
-- counter in its own transaction, so DeleteMedia can refuse a referenced object without a scan.
-- The reconciliation job recomputes the counter from the actual references, because a purge's
-- cascade drops attachment rows without passing through here - the counter is a cache, and the
-- recount is what makes it honest (data-protection.md §5).
UPDATE media_object SET
  ref_count = GREATEST(ref_count + sqlc.arg('delta')::int, 0)
WHERE id = sqlc.arg('id')::uuid;

-- name: DeleteMediaObjectRow :execrows
-- The record's removal, by hand (DeleteMedia): only an unreferenced, live object matches. The
-- bytes are the reconciliation job's to remove - a request has no business waiting on a bucket.
UPDATE media_object SET deleted_at = sqlc.arg('deleted_at')
WHERE id = sqlc.arg('id')::uuid AND ref_count = 0 AND deleted_at IS NULL;

-- name: RecountMediaReferences :exec
-- The reconciliation's first pass: the counter becomes what the references say, and a row that
-- nothing points at learns since when. Whole-tenant, which is a scan by design - the job runs in
-- the background on a bounded table, and a recount that tried to be incremental would be a second
-- bookkeeping to get wrong.
--
-- The stamp is maintained here rather than beside every AdjustRefCount, because this is already
-- the statement that rewrites the counter for every live row: six call sites moving a counter are
-- six places to forget, and this one cannot drift from the count it sets in the same breath.
-- COALESCE keeps the first zero rather than the latest one - a row unreferenced for a week must
-- not have its grace restarted by every pass that walks past it - and a row that gained a
-- reference has its stamp cleared, which is what makes the grace start again when it next loses
-- one.
--
-- READY only, and that is not tidiness. A staging points at nothing by definition, and a stamp it
-- collected while it was PENDING would be an hour old by the time somebody confirmed it late -
-- which would hand the confirmation an object already past its grace. A PENDING row is bounded by
-- its own clock, which runs from the staging; this one starts at the confirmation, where the
-- object first becomes something anything can point at.
UPDATE media_object m SET
  ref_count = c.total,
  unreferenced_since = CASE
    WHEN c.total = 0 AND m.status = 'READY' THEN COALESCE(m.unreferenced_since, sqlc.arg('now'))
    ELSE NULL
  END
FROM (
  SELECT o.id,
    (SELECT count(*) FROM item_attachment a WHERE a.media_id = o.id)
    + (SELECT count(*) FROM work_item w WHERE w.cover_media_id = o.id)
    -- A jumble entry's attachments are references too (G-10): an object a mail brought in must
    -- not be reclaimed while the entry that carries it is still readable.
    + (SELECT count(*) FROM jumble_entry j WHERE o.id = ANY(j.attachments)) AS total
  FROM media_object o
  WHERE o.deleted_at IS NULL
) c
WHERE m.id = c.id AND m.deleted_at IS NULL;

-- name: MarkMediaOrphans :execrows
-- The second pass: what nothing references is marked, not yet removed. Both kinds wait, and they
-- wait on different clocks. A READY object is garbage once nothing has pointed at it for the
-- unreferenced grace - never merely because a pass caught it between its confirmation and the
-- first thing that uses it, which is a window every upload passes through and every detach opens
-- again. A PENDING row is a staging nobody confirmed, and its clock runs from the staging.
--
-- Marking is not a reversible step, which is why the grace sits here rather than after it: a
-- marked object is refused by every read path, so nothing can attach it, so nothing can ever
-- recount it back to life.
UPDATE media_object SET deleted_at = sqlc.arg('now')
WHERE deleted_at IS NULL
  AND ref_count = 0
  AND (
    (status = 'READY'
      AND unreferenced_since IS NOT NULL
      AND unreferenced_since < sqlc.arg('unreferenced_before'))
    OR (status = 'PENDING' AND created_at < sqlc.arg('pending_before'))
  );

-- name: TakeMediaOrphans :many
-- The third pass: marked rows past their grace, handed to the purge job. The keys travel in the
-- job's payload, so the byte deletion needs no transaction of its own to find them.
SELECT id, storage_key
FROM media_object
WHERE deleted_at IS NOT NULL AND deleted_at <= sqlc.arg('marked_before')
ORDER BY deleted_at
LIMIT sqlc.arg('batch');

-- name: RemoveMediaObjectRows :execrows
-- The rows go with the journal entries in one transaction; the bytes follow through the purge
-- job, which retries - at least once, idempotent at the store (core/port/storage).
DELETE FROM media_object WHERE id = ANY(sqlc.arg('ids')::uuid[]);

-- name: ListMediaReferencingItems :many
-- The items an object serves, for GetMedia's authorisation: whoever may read one of them may
-- read the object's record. Bounded - the question is "may this actor see it", and fifty items
-- answer it or nothing will.
SELECT id, collection_id FROM work_item WHERE cover_media_id = sqlc.arg('media_id')::uuid
UNION
SELECT w.id, w.collection_id
FROM item_attachment a JOIN work_item w ON w.id = a.item_id
WHERE a.media_id = sqlc.arg('media_id')::uuid
LIMIT 50;

-- name: InsertItemAttachment :execrows
-- ON CONFLICT DO NOTHING, for the reason AddItemMember gives: attaching what is already attached
-- is the state the caller asked for, and zero rows is how the caller knows not to raise the
-- reference count twice.
INSERT INTO item_attachment (tenant_id, item_id, media_id)
VALUES (current_tenant_id(), sqlc.arg('item_id'), sqlc.arg('media_id'))
ON CONFLICT DO NOTHING;

-- name: DeleteItemAttachment :execrows
DELETE FROM item_attachment
WHERE item_id = sqlc.arg('item_id')::uuid AND media_id = sqlc.arg('media_id')::uuid;

-- name: ListItemAttachmentIDs :many
-- The identifiers alone, ordered stably: what the attach and detach answers carry.
SELECT media_id FROM item_attachment
WHERE item_id = sqlc.arg('item_id')::uuid
ORDER BY media_id;

-- name: ListItemAttachments :many
-- One page of an entry's attachments as media objects, oldest upload first. The keyset is
-- (created_at, id) over the media object, for the reason every list here pages that way
-- (api-guidelines.md §4).
SELECT m.id, m.tenant_id, m.storage_key, m.file_name, m.mime_type, m.byte_size, m.checksum,
       m.usage, m.status, m.ref_count, m.created_by, m.created_at, m.deleted_at
FROM item_attachment a
JOIN media_object m ON m.id = a.media_id
WHERE a.item_id = sqlc.arg('item_id')::uuid
  AND (
    sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (m.created_at, m.id) > (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY m.created_at, m.id
LIMIT sqlc.arg('page_size');
