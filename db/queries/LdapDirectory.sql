-- ============================= LDAP directory ==============================
-- The directory a workspace synchronises from (UC-ID-31, identity.md §27) and whom it holds: the
-- people and groups whose external identifier carries the `ldap:` prefix. What a run changes goes
-- through the directory statements in Identity.sql, as SCIM's acts do.

-- name: FindLdapDirectory :one
SELECT url, bind_dn, password_enc, password_key_id, user_base, user_filter, group_base, group_filter,
       id_attribute, mail_attribute, name_attribute, group_name_attribute, member_attribute, run_as,
       every_minutes, enabled, next_run_at, last_run_at, last_outcome, last_error, last_counts, failures,
       created_at, updated_at
FROM ldap_directory
WHERE tenant_id = current_tenant_id();

-- name: SaveLdapDirectory :exec
-- Set whole; the last run's state stays with the row it describes.
INSERT INTO ldap_directory (
  tenant_id, url, bind_dn, password_enc, password_key_id, user_base, user_filter, group_base,
  group_filter, id_attribute, mail_attribute, name_attribute, group_name_attribute, member_attribute,
  run_as, every_minutes, enabled, next_run_at, created_at, updated_at, updated_by
) VALUES (
  current_tenant_id(), sqlc.arg('url'), sqlc.arg('bind_dn'), sqlc.arg('password_enc'),
  sqlc.arg('password_key_id'), sqlc.arg('user_base'), sqlc.arg('user_filter'), sqlc.narg('group_base'),
  sqlc.arg('group_filter'), sqlc.arg('id_attribute'), sqlc.arg('mail_attribute'),
  sqlc.arg('name_attribute'), sqlc.arg('group_name_attribute'), sqlc.arg('member_attribute'),
  sqlc.arg('run_as'), sqlc.arg('every_minutes'), sqlc.arg('enabled'), sqlc.narg('next_run_at'),
  sqlc.arg('now'), sqlc.arg('now'), sqlc.narg('updated_by')
)
ON CONFLICT (tenant_id) DO UPDATE SET
  url = EXCLUDED.url, bind_dn = EXCLUDED.bind_dn, password_enc = EXCLUDED.password_enc,
  password_key_id = EXCLUDED.password_key_id, user_base = EXCLUDED.user_base,
  user_filter = EXCLUDED.user_filter, group_base = EXCLUDED.group_base,
  group_filter = EXCLUDED.group_filter, id_attribute = EXCLUDED.id_attribute,
  mail_attribute = EXCLUDED.mail_attribute, name_attribute = EXCLUDED.name_attribute,
  group_name_attribute = EXCLUDED.group_name_attribute, member_attribute = EXCLUDED.member_attribute,
  run_as = EXCLUDED.run_as, every_minutes = EXCLUDED.every_minutes, enabled = EXCLUDED.enabled,
  next_run_at = EXCLUDED.next_run_at, failures = 0, updated_at = EXCLUDED.updated_at,
  updated_by = EXCLUDED.updated_by;

-- name: RecordLdapRun :execrows
UPDATE ldap_directory
SET last_run_at = sqlc.arg('last_run_at'), last_outcome = sqlc.arg('last_outcome'),
    last_error = sqlc.narg('last_error'), last_counts = sqlc.arg('last_counts'),
    failures = sqlc.arg('failures'), next_run_at = sqlc.narg('next_run_at')
WHERE tenant_id = current_tenant_id();

-- name: DeleteLdapDirectory :execrows
DELETE FROM ldap_directory WHERE tenant_id = current_tenant_id();

-- name: ListLdapHeldAccounts :many
-- Everybody the directory holds, a restricted person included: the plan decides what is theirs.
SELECT id, external_id::text AS external_id, coalesce(email, '')::text AS email, display_name, status
FROM account
WHERE kind = 'USER' AND deleted_at IS NULL AND status <> 'ANONYMIZED' AND external_id LIKE 'ldap:%'
ORDER BY id;

-- name: ListLdapHeldGroups :many
SELECT id, external_id::text AS external_id, name
FROM account_group
WHERE external_id LIKE 'ldap:%'
ORDER BY id;
