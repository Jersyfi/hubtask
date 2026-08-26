-- The backup targets (E-03, backup-restore.md §2).
--
-- backup_target is behind row level security like every other tenant table, so no statement here
-- carries a tenant condition: the policy applies one no query can forget (ADR-0010). What it means
-- for a target with tenant_id IS NULL - the instance-wide shape 0001_init has always allowed - is
-- that nothing in this milestone can see one, because the policy compares against a tenant and an
-- instance-wide row has none. That is deliberate rather than an oversight: instance administration
-- has no surface on this API yet, and the targets a tenant creates are the tenant's.
--
-- The credential is read by exactly one statement, and it is named for it. Every other statement
-- selects the columns a caller may see, so a credential cannot reach a response because somebody
-- added a field to a mapper.

-- name: InsertBackupTarget :exec
INSERT INTO backup_target (
  id, tenant_id, name, kind, config, credential_enc, credential_key_id,
  encryption_mode, encryption_key_id, region_note, insecure_ack_by, insecure_ack_at,
  enabled, created_at, created_by, version
)
VALUES (
  sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('name'), sqlc.arg('kind'),
  sqlc.arg('config'), sqlc.narg('credential_enc'), sqlc.narg('credential_key_id'),
  sqlc.arg('encryption_mode'), sqlc.narg('encryption_key_id'), sqlc.narg('region_note'),
  sqlc.narg('insecure_ack_by'), sqlc.narg('insecure_ack_at'),
  true, sqlc.arg('created_at'), sqlc.arg('created_by'), 1
);

-- The list a client reads back. Ordered by name rather than by creation, because an operator
-- looking for "the Hetzner one" is looking alphabetically.
-- name: ListBackupTargets :many
SELECT id, tenant_id, name, kind, config, credential_key_id, encryption_mode, encryption_key_id,
       region_note, insecure_ack_by, insecure_ack_at, enabled,
       last_test_at, last_test_ok, last_test_error, created_at, created_by, version
FROM backup_target
ORDER BY lower(name);

-- name: FindBackupTarget :one
SELECT id, tenant_id, name, kind, config, credential_key_id, encryption_mode, encryption_key_id,
       region_note, insecure_ack_by, insecure_ack_at, enabled,
       last_test_at, last_test_ok, last_test_error, created_at, created_by, version
FROM backup_target
WHERE id = sqlc.arg('id');

-- The one statement that reads the sealed credential, and it reads nothing else. A separate
-- statement rather than a column on the two above: what a credential must never do is travel with
-- something that is on its way to a response, and the way to make that structural is for the
-- rows that go to a response never to contain one.
-- name: FindBackupTargetCredential :one
SELECT credential_enc, credential_key_id
FROM backup_target
WHERE id = sqlc.arg('id');

-- What the connection probe leaves behind. The error is a message code, never a driver message:
-- an FTP or SSH library's message carries the host, the user and sometimes the password (rule 10).
-- name: RecordBackupTargetTest :execrows
UPDATE backup_target SET
  last_test_at    = sqlc.arg('tested_at'),
  last_test_ok    = sqlc.arg('succeeded'),
  last_test_error = sqlc.narg('error_code')
WHERE id = sqlc.arg('id');

-- Whether this tenant has any target at all, and whether any of them is unencrypted. Two counts
-- rather than a list, because the health surface asks a question about the installation rather
-- than about a target (backup-restore.md §10).
-- name: CountBackupTargets :one
SELECT
  count(*) AS configured,
  count(*) FILTER (WHERE encryption_mode = 'NONE') AS unencrypted
FROM backup_target
WHERE enabled;
