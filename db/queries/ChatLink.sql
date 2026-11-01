-- A person's chat account linked to a hub's chat bot (migration 0321; UC-SVC-09 check 6).

-- name: StartChatLink :exec
INSERT INTO chat_link (id, tenant_id, bot_id, account_id, code_hash, code_expires_at, created_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('bot_id'), sqlc.arg('account_id'), sqlc.arg('code_hash'),
        sqlc.arg('code_expires_at'), sqlc.arg('created_at'))
ON CONFLICT (tenant_id, bot_id, account_id)
DO UPDATE SET code_hash = EXCLUDED.code_hash, code_expires_at = EXCLUDED.code_expires_at;

-- name: ClaimChatLink :one
-- The code is spent whatever follows; a code run out claims nothing.
UPDATE chat_link
SET code_hash = NULL, code_expires_at = NULL, chat_user = sqlc.arg('chat_user'), address = sqlc.arg('address'),
    linked_at = sqlc.arg('now')
WHERE tenant_id = current_tenant_id() AND bot_id = sqlc.arg('bot_id') AND code_hash = sqlc.arg('code_hash')
  AND code_expires_at > sqlc.arg('now')
RETURNING id, account_id;

-- name: ReleaseChatUser :exec
-- The chat user's earlier link is let go before a claim takes it, in the claim's transaction.
UPDATE chat_link SET chat_user = '', address = '', linked_at = NULL
WHERE tenant_id = current_tenant_id() AND bot_id = sqlc.arg('bot_id') AND chat_user = sqlc.arg('chat_user');

-- name: FindChatLinkByUser :one
-- Only an active account's link answers: a person switched off decides nothing from chat.
SELECT l.id, l.account_id, a.display_name
FROM chat_link l JOIN account a ON a.tenant_id = l.tenant_id AND a.id = l.account_id
WHERE l.tenant_id = current_tenant_id() AND l.bot_id = sqlc.arg('bot_id') AND l.chat_user = sqlc.arg('chat_user')
  AND l.chat_user <> '' AND a.status = 'ACTIVE';

-- name: ListChatLinksOfAccount :many
SELECT id, bot_id, address
FROM chat_link
WHERE tenant_id = current_tenant_id() AND account_id = sqlc.arg('account_id') AND chat_user <> ''
ORDER BY created_at, id;

-- name: FindChatLink :one
SELECT l.id, l.bot_id, l.account_id, l.chat_user, l.address, a.display_name
FROM chat_link l JOIN account a ON a.tenant_id = l.tenant_id AND a.id = l.account_id
WHERE l.tenant_id = current_tenant_id() AND l.id = sqlc.arg('id') AND l.chat_user <> '' AND a.status = 'ACTIVE';

-- name: DeleteChatLink :execrows
DELETE FROM chat_link WHERE tenant_id = current_tenant_id() AND bot_id = sqlc.arg('bot_id') AND account_id = sqlc.arg('account_id');
