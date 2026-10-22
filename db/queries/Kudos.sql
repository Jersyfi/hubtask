-- One person's acknowledgement of another's completion (UC-WRK-29 checks 5 and 6).
--
-- Every statement names the reader's own side: whether they gave kudos, or what they received. No
-- statement counts kudos per person, and none answers who gave kudos to somebody else.

-- name: GiveKudos :exec
-- Idempotent: kudos already given keep their moment.
INSERT INTO kudos (tenant_id, item_id, given_by, given_to, given_at)
VALUES (current_tenant_id(), sqlc.arg('item_id'), sqlc.arg('given_by'), sqlc.arg('given_to'),
        sqlc.arg('given_at'))
ON CONFLICT (tenant_id, item_id, given_by, given_to) DO NOTHING;

-- name: TakeBackKudos :execrows
DELETE FROM kudos
WHERE tenant_id = current_tenant_id() AND item_id = sqlc.arg('item_id')
  AND given_by = sqlc.arg('given_by') AND given_to = sqlc.arg('given_to');

-- name: KudosGiven :one
-- Whether the reader gave kudos to this person's completion of the entry.
SELECT EXISTS (
  SELECT 1 FROM kudos
  WHERE tenant_id = current_tenant_id() AND item_id = sqlc.arg('item_id')
    AND given_by = sqlc.arg('given_by') AND given_to = sqlc.arg('given_to')
);

-- name: KudosReceived :many
-- The kudos the reader received on the entry, oldest first. Bounded: an entry has as many givers as
-- it has readers, and the answer is one person's.
SELECT given_by, given_at FROM kudos
WHERE tenant_id = current_tenant_id() AND item_id = sqlc.arg('item_id')
  AND given_to = sqlc.arg('given_to')
ORDER BY given_at, given_by
LIMIT 500;
