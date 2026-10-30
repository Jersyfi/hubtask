-- A calendar feed pushed into a provider's calendar (migration 0278, UC-INT-16).

-- name: ListCalendarPushes :many
SELECT id, account_id, feed_id, provider, address, state, failure, pushed_at, created_at, updated_at, version
FROM calendar_push
WHERE tenant_id = current_tenant_id() AND account_id = sqlc.arg('account_id')
ORDER BY created_at, id;

-- name: CountCalendarPushes :one
SELECT count(*)::int AS pushes FROM calendar_push
WHERE tenant_id = current_tenant_id() AND account_id = sqlc.arg('account_id');

-- name: FindCalendarPush :one
SELECT id, account_id, feed_id, provider, address, state, failure, pushed_at, created_at, updated_at, version,
       secret_enc, secret_key_id
FROM calendar_push
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: UpsertCalendarPush :exec
INSERT INTO calendar_push (id, tenant_id, account_id, feed_id, provider, address, secret_enc, secret_key_id,
                           state, failure, pushed_at, created_at, updated_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('feed_id'), sqlc.arg('provider'),
        sqlc.arg('address'), sqlc.arg('secret_enc'), sqlc.arg('secret_key_id'), 'ACTIVE', '', NULL,
        sqlc.arg('created_at'), sqlc.arg('created_at'), 1)
ON CONFLICT (tenant_id, feed_id, provider) DO UPDATE
SET address = EXCLUDED.address, secret_enc = EXCLUDED.secret_enc, secret_key_id = EXCLUDED.secret_key_id,
    state = 'ACTIVE', failure = '', updated_at = EXCLUDED.updated_at, version = calendar_push.version + 1;

-- name: FindCalendarPushByFeed :one
SELECT id FROM calendar_push
WHERE tenant_id = current_tenant_id() AND feed_id = sqlc.arg('feed_id') AND provider = sqlc.arg('provider');

-- name: UpdateCalendarPushRun :exec
UPDATE calendar_push
SET state = sqlc.arg('state'), failure = sqlc.arg('failure'), pushed_at = sqlc.narg('pushed_at'),
    updated_at = sqlc.arg('updated_at'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: UpdateCalendarPushSecret :exec
UPDATE calendar_push SET secret_enc = sqlc.arg('secret_enc'), secret_key_id = sqlc.arg('secret_key_id')
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: DeleteCalendarPush :execrows
DELETE FROM calendar_push WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: ListCalendarPushEvents :many
SELECT entry_id, remote_id, digest FROM calendar_push_event
WHERE tenant_id = current_tenant_id() AND push_id = sqlc.arg('push_id')
ORDER BY entry_id;

-- name: UpsertCalendarPushEvent :exec
INSERT INTO calendar_push_event (tenant_id, push_id, entry_id, remote_id, digest, pushed_at)
VALUES (current_tenant_id(), sqlc.arg('push_id'), sqlc.arg('entry_id'), sqlc.arg('remote_id'), sqlc.arg('digest'), sqlc.arg('pushed_at'))
ON CONFLICT (tenant_id, push_id, entry_id) DO UPDATE
SET remote_id = EXCLUDED.remote_id, digest = EXCLUDED.digest, pushed_at = EXCLUDED.pushed_at;

-- name: DeleteCalendarPushEvent :exec
DELETE FROM calendar_push_event
WHERE tenant_id = current_tenant_id() AND push_id = sqlc.arg('push_id') AND entry_id = sqlc.arg('entry_id');

-- name: InsertCalendarPushAuthorization :exec
INSERT INTO calendar_push_authorization (id, tenant_id, account_id, feed_id, provider, state_hash, verifier_enc, verifier_key_id, expires_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('feed_id'), sqlc.arg('provider'),
        sqlc.arg('state_hash'), sqlc.arg('verifier_enc'), sqlc.arg('verifier_key_id'), sqlc.arg('expires_at'));

-- name: TakeCalendarPushAuthorization :one
DELETE FROM calendar_push_authorization
WHERE tenant_id = current_tenant_id() AND state_hash = sqlc.arg('state_hash')
RETURNING id, account_id, feed_id, provider, verifier_enc, verifier_key_id, expires_at;

-- name: DeleteExpiredCalendarPushAuthorizations :execrows
DELETE FROM calendar_push_authorization WHERE tenant_id = current_tenant_id() AND expires_at <= sqlc.arg('now');
