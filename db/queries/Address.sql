-- A managed account takes an address (ADR-0074 §6): the address waits on an ADDRESS row of
-- auth_pending until somebody holding the mailbox follows the link; only then is it the account's.

-- name: LockAccountForAddress :one
-- The account's row, locked for the request's transaction: two requests for one account are
-- serialised, so the ten-minute floor between two mails is read and kept by one of them at a time.
SELECT id, email, sign_in_name, status
FROM account
WHERE id = sqlc.arg('account_id') AND deleted_at IS NULL
FOR UPDATE;

-- name: LatestAddressRequest :one
-- When the newest request for this account was written, spent or not: the floor counts mails.
SELECT created_at
FROM auth_pending
WHERE account_id = sqlc.arg('account_id') AND purpose = 'ADDRESS'
ORDER BY created_at DESC
LIMIT 1;

-- name: InsertAddressRequest :exec
-- No token yet: it is minted when the mail goes out, so the plaintext exists only in the message.
INSERT INTO auth_pending (id, tenant_id, account_id, purpose, address, created_at, expires_at)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), 'ADDRESS', sqlc.arg('address'),
  sqlc.arg('created_at'), sqlc.arg('expires_at')
);

-- name: MintAddressToken :one
-- The mail's token, for a request that is neither spent nor expired. A retried delivery mints
-- again, and only the newest mail's link works - the reset link's rule.
UPDATE auth_pending p
   SET token_hash = sqlc.arg('token_hash')
  FROM account a
 WHERE p.id = sqlc.arg('id') AND p.purpose = 'ADDRESS'
   AND p.consumed_at IS NULL AND p.expires_at > sqlc.arg('now')
   AND a.id = p.account_id AND a.deleted_at IS NULL
RETURNING p.address::text AS address, a.display_name, a.locale;

-- name: FindAddressRequestByHash :one
-- The confirmation's read: the request, its account's state and the workspace's.
SELECT p.id, p.account_id, p.address::text AS address, p.created_at, p.expires_at, p.consumed_at,
       a.status AS account_status, n.status::text AS tenant_status
FROM auth_pending p
JOIN account a ON a.id = p.account_id
JOIN tenant  n ON n.id = p.tenant_id
WHERE p.token_hash = sqlc.arg('token_hash') AND p.purpose = 'ADDRESS' AND a.deleted_at IS NULL;

-- name: ConfirmAccountAddress :execrows
-- The address becomes the account's, while it is still a managed one that can act: an erased
-- account, or one that has an address by now, takes none (zero rows). An address another account
-- holds meanwhile is refused by account_email_uq.
UPDATE account
   SET email = sqlc.arg('address'),
       updated_at = sqlc.arg('at'),
       version = version + 1
 WHERE id = sqlc.arg('account_id') AND deleted_at IS NULL
   AND sign_in_name IS NOT NULL AND email IS NULL AND status <> 'ANONYMIZED';

-- name: DeleteAccountPending :execrows
-- An erasure takes the account's unfinished steps with it, an address waiting for its confirmation
-- among them.
DELETE FROM auth_pending WHERE account_id = sqlc.arg('account_id');
