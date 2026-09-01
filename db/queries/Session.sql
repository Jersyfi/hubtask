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
       n.default_locale, n.default_time_zone,
       n.slug AS tenant_slug, n.status::text AS tenant_status
FROM account a
JOIN tenant n ON n.id = a.tenant_id
WHERE lower(a.email) = lower(sqlc.arg('email')) AND a.deleted_at IS NULL;

-- ============================== Sessions ==============================

-- name: InsertSession :exec
-- grant_id and scopes are H-05's leash: set for a session an OAuth exchange issued, NULL for a
-- person's own.
INSERT INTO session
  (id, tenant_id, account_id, created_at, user_agent, ip_class, expires_at, grant_id, scopes)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('created_at'),
  sqlc.narg('user_agent'), sqlc.narg('ip_class'), sqlc.arg('expires_at'),
  sqlc.narg('grant_id'), sqlc.narg('scopes')
);

-- name: FindSessionForAuth :one
-- What authenticating a session access token needs: the row the signature named, its account,
-- and the locale chain - one round trip, the FindAccessTokenByHash shape.
SELECT s.id, s.tenant_id, s.account_id, s.created_at, s.last_seen_at, s.expires_at, s.revoked_at,
       s.grant_id, s.scopes,
       g.client_id AS grant_client_id,
       a.kind     AS account_kind,
       a.status   AS account_status,
       a.display_name AS account_display_name,
       a.locale   AS account_locale,
       a.time_zone AS account_time_zone,
       n.default_locale, n.default_time_zone,
       n.slug AS tenant_slug, n.status::text AS tenant_status
FROM session s
JOIN account a ON a.id = s.account_id
JOIN tenant  n ON n.id = s.tenant_id
LEFT JOIN oauth_grant g ON g.id = s.grant_id
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
       n.default_locale, n.default_time_zone,
       n.slug AS tenant_slug, n.status::text AS tenant_status
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
       n.default_locale, n.default_time_zone,
       n.slug AS tenant_slug, n.status::text AS tenant_status
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

-- ============================ The second factor (H-02) ============================

-- name: UpsertMfaEnrollment :execrows
-- A fresh enrolment, or the replacement of an unconfirmed one. An armed enrolment matches
-- nothing - zero rows is the "disable first, with the password" refusal - so a stolen session
-- cannot quietly swap the secret out from under the real authenticator.
INSERT INTO account_mfa
  (account_id, tenant_id, secret_enc, secret_key_id, created_at, updated_at)
VALUES (
  sqlc.arg('account_id'), current_tenant_id(), sqlc.arg('secret_enc'),
  sqlc.arg('secret_key_id'), sqlc.arg('now'), sqlc.arg('now')
)
ON CONFLICT (account_id) DO UPDATE
SET secret_enc = EXCLUDED.secret_enc,
    secret_key_id = EXCLUDED.secret_key_id,
    confirmed_at = NULL,
    last_step = NULL,
    updated_at = EXCLUDED.updated_at
WHERE account_mfa.confirmed_at IS NULL;

-- name: FindMfaEnrollment :one
SELECT account_id, secret_enc, secret_key_id, confirmed_at, last_step
FROM account_mfa
WHERE account_id = sqlc.arg('account_id');

-- name: ConfirmMfaEnrollment :execrows
-- Arms the enrolment and records the confirming step in one statement, so the code that armed
-- can never verify a second time.
UPDATE account_mfa SET confirmed_at = sqlc.arg('now'), last_step = sqlc.arg('step'),
  updated_at = sqlc.arg('now')
WHERE account_id = sqlc.arg('account_id') AND confirmed_at IS NULL;

-- name: RecordMfaStep :execrows
-- The replay refusal, atomically: only a step past the last accepted one writes, and zero rows
-- means the same or an older code was presented again.
UPDATE account_mfa SET last_step = sqlc.arg('step'), updated_at = sqlc.arg('now')
WHERE account_id = sqlc.arg('account_id')
  AND confirmed_at IS NOT NULL
  AND (last_step IS NULL OR last_step < sqlc.arg('step'));

-- name: DisableMfa :execrows
-- The enrolment goes whole; the recovery codes go with it by their own statement in the same
-- transaction, because half a disable is worse than none.
DELETE FROM account_mfa WHERE account_id = sqlc.arg('account_id');

-- name: DeleteRecoveryCodes :execrows
DELETE FROM account_recovery_code WHERE account_id = sqlc.arg('account_id');

-- name: InsertRecoveryCode :exec
-- The hash is computed in the adapter, the pepper's home (security.md §8).
INSERT INTO account_recovery_code (id, tenant_id, account_id, code_hash, created_at)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'),
  sqlc.arg('code_hash'), sqlc.arg('created_at')
);

-- name: BurnRecoveryCode :execrows
-- Only the first use writes; a code presented again matches nothing, which is what single-use
-- means at this layer.
UPDATE account_recovery_code SET used_at = sqlc.arg('now')
WHERE account_id = sqlc.arg('account_id')
  AND code_hash = sqlc.arg('code_hash')
  AND used_at IS NULL;

-- name: CountRecoveryCodes :one
SELECT count(*) FROM account_recovery_code
WHERE account_id = sqlc.arg('account_id') AND used_at IS NULL;

-- ====================== The pending credential (H-02) ======================

-- name: InsertPendingCredential :exec
INSERT INTO auth_pending
  (id, tenant_id, account_id, token_hash, purpose, user_agent, ip_class, created_at, expires_at)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('token_hash'),
  sqlc.arg('purpose'), sqlc.narg('user_agent'), sqlc.narg('ip_class'),
  sqlc.arg('created_at'), sqlc.arg('expires_at')
);

-- name: FindPendingByHash :one
-- The second step's read: the pending row, its account, and the locale chain in one round trip,
-- FindSessionForAuth's shape.
SELECT p.id, p.account_id, p.purpose, p.user_agent, p.ip_class,
       p.created_at, p.expires_at, p.consumed_at,
       a.kind     AS account_kind,
       a.status   AS account_status,
       a.display_name AS account_display_name,
       a.locale   AS account_locale,
       a.time_zone AS account_time_zone,
       n.default_locale, n.default_time_zone,
       n.slug AS tenant_slug, n.status::text AS tenant_status
FROM auth_pending p
JOIN account a ON a.id = p.account_id
JOIN tenant  n ON n.id = p.tenant_id
WHERE p.token_hash = sqlc.arg('token_hash') AND a.deleted_at IS NULL;

-- name: ConsumePendingCredential :execrows
-- Single use, atomically: only the first completion writes, and a lost race answers exactly as
-- an unknown token does.
UPDATE auth_pending SET consumed_at = sqlc.arg('now')
WHERE id = sqlc.arg('id') AND consumed_at IS NULL;

-- name: DeleteExpiredPending :execrows
-- Hygiene in the session sweep's pass: a pending row lives minutes, and one that outlived them
-- is bookkeeping about a sign-in nobody finished.
DELETE FROM auth_pending
WHERE id IN (
  SELECT id FROM auth_pending AS expired
  WHERE expired.expires_at < sqlc.arg('cutoff')
  LIMIT sqlc.arg('batch')
);

-- name: TenantSettings :one
-- The tenant's own row, reachable under its own policy: the enforcement switch lives in the
-- settings document (multi-tenancy.md §4's home for per-tenant knobs).
SELECT settings FROM tenant WHERE id = current_tenant_id();

-- name: FindPasswordHash :one
-- For the operations that demand the password afresh of somebody already signed in (H-02):
-- disabling the second factor is the attack a stolen session would try, and a live session is
-- deliberately not enough there.
SELECT password_hash FROM account
WHERE id = sqlc.arg('id') AND deleted_at IS NULL;

-- ============================ The step-up (H-03) ============================

-- name: RecordStepUp :execrows
-- The proof lands on the caller's own session, replacing whatever stood: a fresh proof is the
-- newest answer to "is this still you", and two live proofs would be two coverings.
UPDATE session SET
  step_up_token_hash  = sqlc.arg('token_hash'),
  step_up_at          = sqlc.arg('now'),
  step_up_method      = sqlc.arg('method'),
  step_up_consumed_at = NULL
WHERE id = sqlc.arg('id')
  AND account_id = sqlc.arg('account_id')
  AND revoked_at IS NULL;

-- name: ConsumeStepUp :execrows
-- The one statement the whole feature turns on: the proof is judged and burned atomically -
-- fresh, unconsumed, on a live session of this account - so two privileged actions racing for
-- one proof settle in the database, not in Go. Zero rows is "not proved", whatever the reason.
UPDATE session SET step_up_consumed_at = sqlc.arg('now')
WHERE step_up_token_hash = sqlc.arg('token_hash')
  AND account_id = sqlc.arg('account_id')
  AND step_up_consumed_at IS NULL
  AND step_up_at >= sqlc.arg('cutoff')
  AND revoked_at IS NULL
  AND expires_at > sqlc.arg('now');

-- name: FindStepUpMethod :one
-- What proved it, for the audit entry - never the credential.
SELECT step_up_method FROM session
WHERE step_up_token_hash = sqlc.arg('token_hash') AND account_id = sqlc.arg('account_id');
