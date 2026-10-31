-- A chat bot in a hub's room (migrations 0280, 0288; UC-INT-18, UC-INT-19).

-- name: ListChatBots :many
SELECT id, hub_id, collection_id, platform, homeserver, room, bot_user, locale, unfurl, capture, enabled, run_as, since,
       failure, created_at, updated_at, version
FROM chat_bot
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id')
ORDER BY created_at, id;

-- name: CountChatBots :one
SELECT count(*)::int AS bots FROM chat_bot WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id');

-- name: FindChatBot :one
SELECT id, hub_id, collection_id, platform, homeserver, room, bot_user, locale, unfurl, capture, enabled, run_as, since,
       failure, created_at, updated_at, version, secret_enc, secret_key_id
FROM chat_bot
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: InsertChatBot :exec
INSERT INTO chat_bot (id, tenant_id, hub_id, collection_id, platform, homeserver, room, bot_user, locale, unfurl, capture,
                      enabled, run_as, since, failure, secret_enc, secret_key_id, created_at, updated_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('hub_id'), sqlc.arg('collection_id'), sqlc.arg('platform'), sqlc.arg('homeserver'),
        sqlc.arg('room'), sqlc.arg('bot_user'), sqlc.arg('locale'), sqlc.arg('unfurl'), sqlc.arg('capture'), true,
        sqlc.arg('run_as'), '', '', sqlc.arg('secret_enc'), sqlc.arg('secret_key_id'), sqlc.arg('created_at'),
        sqlc.arg('created_at'), 1);

-- name: UpdateChatBot :exec
UPDATE chat_bot
SET collection_id = sqlc.arg('collection_id'), locale = sqlc.arg('locale'), unfurl = sqlc.arg('unfurl'),
    capture = sqlc.arg('capture'), enabled = sqlc.arg('enabled'), run_as = sqlc.arg('run_as'),
    updated_at = sqlc.arg('updated_at'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: RecordChatBotRun :exec
UPDATE chat_bot SET since = sqlc.arg('since'), failure = sqlc.arg('failure'), updated_at = sqlc.arg('updated_at')
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: DeleteChatBot :execrows
DELETE FROM chat_bot WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');
