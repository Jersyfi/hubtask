-- The sign-in surface (H-01, security.md §5): sessions, refresh rotation, the attempt ledger and
-- the invitation redeemed.
--
-- The tenant is never a parameter in this file: row level security bounds every statement to the
-- tenant of the running transaction, which is what makes a session of another workspace invisible
-- rather than forbidden (ADR-0010, multi-tenancy.md §2). The one exception is ResolveTenant,
-- which exists because sign-in needs a tenant before it can open a bounded transaction at all
-- (0.6.0 decision 3) - it answers one identifier or none, through the SECURITY DEFINER function
-- migration 0063 pins down, and never a listing.

-- name: ResolveTenant :one
SELECT resolve_tenant(sqlc.narg('slug'))::uuid AS tenant_id;

-- ============================== Sign-in ==============================

-- name: FindAccountForSignIn :one
-- The credential check's read: the stored hash beside everything the session will need, in one
-- round trip. Compared lower case, the way the uniqueness index does (account_email_uq).
SELECT a.id, a.kind, a.email, a.display_name, a.status,
       a.locale AS account_locale, a.time_zone AS account_time_zone,
       a.password_hash,
       n.default_locale, n.default_time_zone
FROM account a
JOIN tenant n ON n.id = a.tenant_id
WHERE lower(a.email) = lower(sqlc.arg('email')) AND a.deleted_at IS NULL;

-- ============================== Sessions ==============================

-- name: InsertSession :exec
INSERT INTO session (id, tenant_id, account_id, created_at, user_agent, ip_class, expires_at)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('created_at'),
  sqlc.narg('user_agent'), sqlc.narg('ip_class'), sqlc.arg('expires_at')
);

-- name: FindSessionForAuth :one
-- What authenticating a session access token needs: the row the signature named, its account,
-- and the locale chain - one round trip, the FindAccessTokenByHash shape.
SELECT s.id, s.tenant_id, s.account_id, s.created_at, s.last_seen_at, s.expires_at, s.revoked_at,
       a.kind     AS account_kind,
       a.status   AS account_status,
       a.display_name AS account_display_name,
       a.locale   AS account_locale,
       a.time_zone AS account_time_zone,
       n.default_locale, n.default_time_zone
FROM session s
JOIN account a ON a.id = s.account_id
JOIN tenant  n ON n.id = s.tenant_id
WHERE s.id = sqlc.arg('id') AND a.deleted_at IS NULL;

-- name: SessionsForAccount :many
-- One's own live sessions, newest first. The dead ones are deliberately absent: a listing is for
-- deciding what to end, and what is already ended or run out is nothing anybody can act on.
SELECT id, account_id, created_at, last_seen_at, user_agent, ip_class, expires_at, revoked_at
FROM session
WHERE account_id = sqlc.arg('account_id')
  AND revoked_at IS NULL
  AND expires_at > sqlc.arg('now')
ORDER BY created_at DESC, id DESC;

-- name: TouchSession :exec
UPDATE session SET last_seen_at = $2 WHERE id = $1;

-- name: ExtendSession :exec
-- Rotation slides the horizon: the session lives as long as its newest refresh token could.
UPDATE session SET expires_at = sqlc.arg('expires_at') WHERE id = sqlc.arg('id');

-- name: RevokeSession :execrows
-- Bounded to the owner in the same statement that writes, and only the first withdrawal writes -
-- a second call matches nothing, which is how the use case tells "ended just now" from "already
-- ended" without reading the row again.
UPDATE session SET revoked_at = sqlc.arg('revoked_at')
WHERE id = sqlc.arg('id') AND account_id = sqlc.arg('account_id') AND revoked_at IS NULL;

-- name: RevokeAllSessionsForAccount :execrows
-- Every device at once, the answer to "I left myself signed in somewhere".
UPDATE session SET revoked_at = sqlc.arg('revoked_at')
WHERE account_id = sqlc.arg('account_id') AND revoked_at IS NULL;

-- ============================ Refresh tokens ============================

-- name: InsertRefreshToken :exec
-- The hash is computed in the adapter, because the pepper is a secret of that layer and the
-- application must never hold a value it could store by mistake (security.md §8).
INSERT INTO session_refresh_token (id, tenant_id, session_id, token_hash, created_at, expires_at)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('session_id'), sqlc.arg('token_hash'),
  sqlc.arg('created_at'), sqlc.arg('expires_at')
);

-- name: FindRefreshTokenByHash :one
-- The exchange's read: the presented token, its session, the account and the locale chain in one
-- round trip. The rotated ones are found on purpose - a rotated hash presented again is the
-- reuse signal T-01 exists for, and the caller has to be able to tell it from an unknown token.
SELECT r.id, r.session_id, r.created_at, r.expires_at, r.rotated_at,
       s.tenant_id,
       s.account_id,
       s.created_at   AS session_created_at,
       s.last_seen_at AS session_last_seen_at,
       s.user_agent   AS session_user_agent,
       s.ip_class     AS session_ip_class,
       s.expires_at   AS session_expires_at,
       s.revoked_at   AS session_revoked_at,
       a.kind     AS account_kind,
       a.status   AS account_status,
       a.display_name AS account_display_name,
       a.locale   AS account_locale,
       a.time_zone AS account_time_zone,
       n.default_locale, n.default_time_zone
FROM session_refresh_token r
JOIN session s ON s.id = r.session_id
JOIN account a ON a.id = s.account_id
JOIN tenant  n ON n.id = r.tenant_id
WHERE r.token_hash = $1 AND a.deleted_at IS NULL;

-- name: RotateRefreshToken :execrows
-- Only the first exchange writes. A second call matches nothing, and that nothing is the reuse
-- detection: the caller then kills the family rather than minting a second line.
UPDATE session_refresh_token SET rotated_at = sqlc.arg('rotated_at')
WHERE id = sqlc.arg('id') AND rotated_at IS NULL;

-- ========================= The attempt ledger (T-02) =========================

-- name: FindAuthAttempt :one
SELECT failures, last_failure_at, locked_until
FROM auth_attempt
WHERE subject_hash = sqlc.arg('subject_hash');

-- name: UpsertAuthAttempt :exec
-- The counter and the moment are computed by the caller from what it read: the delay curve is the
-- domain's, and a statement that computed it would be policy in SQL.
INSERT INTO auth_attempt (tenant_id, subject_hash, failures, last_failure_at, locked_until)
VALUES (
  current_tenant_id(), sqlc.arg('subject_hash'), sqlc.arg('failures'),
  sqlc.arg('last_failure_at'), sqlc.narg('locked_until')
)
ON CONFLICT (tenant_id, subject_hash) DO UPDATE
SET failures        = EXCLUDED.failures,
    last_failure_at = EXCLUDED.last_failure_at,
    locked_until    = EXCLUDED.locked_until;

-- name: ClearAuthAttempt :exec
-- A successful sign-in wipes the slate: the ledger exists to slow guessing, not to remember it.
DELETE FROM auth_attempt WHERE subject_hash = sqlc.arg('subject_hash');

-- ========================== Invitation redemption ==========================

-- name: SetRedemptionToken :execrows
-- Minted on invite and replaced by re-inviting; only an account still waiting can carry one.
UPDATE account SET
  redemption_token_hash = sqlc.arg('token_hash'),
  redemption_expires_at = sqlc.arg('expires_at'),
  updated_at            = sqlc.arg('now')
WHERE id = sqlc.arg('id') AND status = 'INVITED' AND deleted_at IS NULL;

-- name: FindAccountByRedemptionHash :one
SELECT a.id, a.kind, a.email, a.display_name, a.status,
       a.locale AS account_locale, a.time_zone AS account_time_zone,
       a.redemption_expires_at,
       n.default_locale, n.default_time_zone
FROM account a
JOIN tenant n ON n.id = a.tenant_id
WHERE a.redemption_token_hash = sqlc.arg('token_hash') AND a.deleted_at IS NULL;

-- name: RedeemInvitation :execrows
-- One statement for the whole act: the password lands, the account becomes ACTIVE, and the token
-- dies - so a second redemption matches nothing however fresh the token looked a moment ago.
UPDATE account SET
  password_hash         = sqlc.arg('password_hash'),
  status                = 'ACTIVE',
  redemption_token_hash = NULL,
  redemption_expires_at = NULL,
  updated_at            = sqlc.arg('now'),
  version               = version + 1
WHERE id = sqlc.arg('id')
  AND status = 'INVITED'
  AND redemption_token_hash IS NOT NULL
  AND deleted_at IS NULL;

-- ========================= The retention sweep =========================

-- name: DeleteExpiredSessions :execrows
-- The SESSION data kind's sweep (data-retention.md §3: anchor `last_seen_at`, 30 days). Only a
-- session that is already over - run out or revoked - ages out: the anchor decides *when* the row
-- goes, never whether a live sign-in ends, because ending sign-ins is revocation's job and the
-- engine's job is forgetting. The refresh family goes with the row by cascade.
--
-- Batched through a subquery, because DELETE takes no LIMIT: a pass that took every expired row
-- would be a pass nobody can stop. Oldest first, so a backlog drains in the order it built up.
DELETE FROM session
WHERE id IN (
  SELECT id FROM session AS expired
  WHERE coalesce(expired.last_seen_at, expired.created_at) < sqlc.arg('cutoff')
    AND (expired.expires_at < sqlc.arg('cutoff') OR expired.revoked_at IS NOT NULL)
  ORDER BY coalesce(expired.last_seen_at, expired.created_at)
  LIMIT sqlc.arg('batch')
);

-- name: CountExpiredSessions :one
-- What is due, so a pass can report a backlog it did not get to. Bounded by the batch it would
-- have taken plus one, so a tenant with a million expired rows costs an index scan of a page
-- rather than a count of the table.
SELECT count(*) FROM (
  SELECT 1 FROM session AS expired
  WHERE coalesce(expired.last_seen_at, expired.created_at) < sqlc.arg('cutoff')
    AND (expired.expires_at < sqlc.arg('cutoff') OR expired.revoked_at IS NOT NULL)
  LIMIT sqlc.arg('ceiling')
) AS due;
