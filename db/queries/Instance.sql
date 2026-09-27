-- name: ReadInstanceSettings :many
-- The installation's own level (ADR-0070 §2).
--
-- The whole set in one read, because it is small by construction - eighteen switches and four
-- links - and because the resolver needs all of it to answer one password screen. No tenant
-- predicate: the table deliberately carries no policy, and the transaction reading it has no
-- tenant at all.
SELECT key, value, lock_origin, updated_by, updated_at
FROM instance_setting
ORDER BY key;

-- name: PutInstanceSetting :exec
-- One switch, written or overwritten.
INSERT INTO instance_setting (key, value, lock_origin, updated_by, updated_at)
VALUES (
  sqlc.arg('key'), sqlc.arg('value'), sqlc.arg('lock_origin'),
  sqlc.narg('updated_by'), sqlc.arg('updated_at')
)
ON CONFLICT (key) DO UPDATE
  SET value = excluded.value,
      lock_origin = excluded.lock_origin,
      updated_by = excluded.updated_by,
      updated_at = excluded.updated_at;

-- name: DeleteInstanceSettingsExcept :exec
-- What the write did not decide stops being decided.
--
-- In the same transaction as the writes, which is what makes `PUT` mean what it says: a switch the
-- operator cleared is gone rather than left standing at its old value. Bounded to the areas the
-- caller wrote, so that a future area of settings is not emptied by a write to this one.
DELETE FROM instance_setting
WHERE split_part(key, '.', 1) = ANY(sqlc.arg('areas')::text[])
  AND NOT (key = ANY(sqlc.arg('kept')::text[]));
