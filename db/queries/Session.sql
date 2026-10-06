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
  (id, tenant_id, account_id, created_at, user_agent, ip_class, expires_at, grant_id, scopes,
   hard_expires_at, idle_minutes, signed_in_with, signed_in_provider_id)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('created_at'),
  sqlc.narg('user_agent'), sqlc.narg('ip_class'), sqlc.arg('expires_at'),
  sqlc.narg('grant_id'), sqlc.narg('scopes'),
  -- The session's own bounds and how it was opened (migration 0100, ADR-0068 §3).
  sqlc.narg('hard_expires_at'), sqlc.narg('idle_minutes'), sqlc.narg('signed_in_with'),
  -- Which provider opened it, for one opened through a provider (migration 0111).
  sqlc.narg('signed_in_provider_id')
);

-- name: FindSessionForAuth :one
-- What authenticating a session access token needs: the row the signature named, its account,
-- and the locale chain - one round trip, the FindAccessTokenByHash shape.
SELECT s.id, s.tenant_id, s.account_id, s.created_at, s.last_seen_at, s.expires_at, s.revoked_at,
       s.grant_id, s.scopes,
       s.hard_expires_at, s.idle_minutes, s.signed_in_with, s.elevated_until,
       -- The rotation cutoff (ADR-0068 §3), off the row this query already joins: a session opened
       -- before the moment somebody asked everybody for a new password is refused on its next
       -- request. One extraction on a row already in hand, which is what makes the enforcement cost
       -- nothing per request.
       (n.settings #>> '{sign_in_policy,rotation_from}') AS rotation_from,
       g.client_id AS grant_client_id,
       a.kind     AS account_kind,
       a.status   AS account_status,
       a.display_name AS account_display_name,
       a.locale   AS account_locale,
       a.time_zone AS account_time_zone,
       a.week_start AS account_week_start,
       n.default_locale, n.default_time_zone,
       n.slug AS tenant_slug, n.status::text AS tenant_status,
       coalesce((n.settings #>> '{quotas,api_requests_per_minute}')::bigint, 0) AS token_rate_override
FROM session s
JOIN account a ON a.id = s.account_id
JOIN tenant  n ON n.id = s.tenant_id
LEFT JOIN oauth_grant g ON g.id = s.grant_id
WHERE s.id = sqlc.arg('id') AND a.deleted_at IS NULL;

-- name: SessionsForAccount :many
-- One's own unrevoked, unexpired sessions, newest first. The ended and the run-out are absent here;
-- the workspace's bounds are judged by the application with the method authentication uses (SC-19),
-- which is why the rotation cutoff rides along, off the row FindSessionForAuth reads it from.
--
-- The provider's name is joined under the reader's own row policy (migration 0111): one this tenant
-- can see - its own or the installation's - is named, and a removed one or another tenant's is not.
SELECT s.id, s.account_id, s.created_at, s.last_seen_at, s.user_agent, s.ip_class, s.expires_at,
       s.revoked_at, s.hard_expires_at, s.idle_minutes, s.signed_in_with, s.signed_in_provider_id,
       p.display_name AS signed_in_provider_name,
       (n.settings #>> '{sign_in_policy,rotation_from}') AS rotation_from
FROM session s
JOIN tenant n ON n.id = s.tenant_id
LEFT JOIN identity_provider p ON p.id = s.signed_in_provider_id
WHERE s.account_id = sqlc.arg('account_id')
  AND s.revoked_at IS NULL
  AND s.expires_at > sqlc.arg('now')
ORDER BY s.created_at DESC, s.id DESC;

-- name: TouchSession :exec
UPDATE session SET last_seen_at = $2 WHERE id = $1;

-- name: ElevateSession :execrows
-- Raises one live session of the account to the control plane's scope for a bounded while
-- (ADR-0070 §4). Bounded to the owner in the same statement that writes, RevokeSession's discipline:
-- a session that is not the caller's matches nothing rather than being refused after a read.
UPDATE session SET elevated_until = sqlc.arg('elevated_until')
WHERE id = sqlc.arg('id') AND account_id = sqlc.arg('account_id')
  AND revoked_at IS NULL AND expires_at > sqlc.arg('now');

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

-- name: RevokeOtherSessionsForAccount :execrows
-- Every device but the one asking (UC-ID-06 check 4). A NULL `keep` spares nothing: the comparison
-- is written so that it can never become `id <> ''`, which would compare against nothing at all.
UPDATE session SET revoked_at = sqlc.arg('revoked_at')
WHERE account_id = sqlc.arg('account_id') AND revoked_at IS NULL
  AND (sqlc.narg('keep')::uuid IS NULL OR id <> sqlc.narg('keep')::uuid);

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

-- name: CountAuthFailure :one
-- One more failure, added where the row is rather than computed from a read: the row is held until
-- the transaction ends, so twenty guesses sent at once are twenty failures, not one written twenty
-- times. The lock moment is the caller's to compute from the count this answers (UpsertAuthAttempt).
INSERT INTO auth_attempt (tenant_id, subject_hash, failures, last_failure_at)
VALUES (current_tenant_id(), sqlc.arg('subject_hash'), 1, sqlc.arg('at'))
ON CONFLICT (tenant_id, subject_hash) DO UPDATE
SET failures        = auth_attempt.failures + 1,
    last_failure_at = EXCLUDED.last_failure_at
RETURNING failures;

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
  -- The moment the password was set, beside the hash (migration 0098): what `max_age_days` and
  -- `rotation_from` are compared against, written here so a first password has one from its first
  -- minute rather than from whenever it is next changed.
  password_set_at       = sqlc.arg('now'),
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
SELECT account_id, secret_enc, secret_key_id, confirmed_at, last_step,
       replacement_secret_enc, replacement_secret_key_id, replacement_session_id, replacement_expires_at
FROM account_mfa
WHERE account_id = sqlc.arg('account_id');

-- name: StartMfaReplacement :execrows
-- The new secret beside the armed one (SC-17): only an armed enrolment takes a replacement - an
-- unconfirmed one is replaced by enrolling again - and a replacement begun earlier is overwritten,
-- the latest start being the one the person is looking at.
UPDATE account_mfa SET
  replacement_secret_enc    = sqlc.arg('secret_enc'),
  replacement_secret_key_id = sqlc.arg('secret_key_id'),
  replacement_session_id    = sqlc.arg('session_id'),
  replacement_expires_at    = sqlc.arg('expires_at'),
  updated_at                = sqlc.arg('now')
WHERE account_id = sqlc.arg('account_id') AND confirmed_at IS NOT NULL;

-- name: SwapMfaReplacement :execrows
-- The swap, in one statement: the replacement becomes the armed secret and the old one is gone, with
-- the confirming step as the new replay floor. Only for the session that began it, inside its window,
-- and only if the replacement is still the very secret the confirmation verified - a second start in
-- between would otherwise arm a secret nobody proved.
UPDATE account_mfa SET
  secret_enc                = replacement_secret_enc,
  secret_key_id             = replacement_secret_key_id,
  last_step                 = sqlc.arg('step'),
  replacement_secret_enc    = NULL,
  replacement_secret_key_id = NULL,
  replacement_session_id    = NULL,
  replacement_expires_at    = NULL,
  updated_at                = sqlc.arg('now')
WHERE account_id = sqlc.arg('account_id')
  AND confirmed_at IS NOT NULL
  AND replacement_session_id = sqlc.arg('session_id')
  AND replacement_expires_at > sqlc.arg('now')
  AND replacement_secret_enc = sqlc.arg('expected_secret_enc');

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
  (id, tenant_id, account_id, token_hash, purpose, user_agent, ip_class, created_at, expires_at,
   link_provider_id, link_subject, link_proof)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('token_hash'),
  sqlc.arg('purpose'), sqlc.narg('user_agent'), sqlc.narg('ip_class'),
  sqlc.arg('created_at'), sqlc.arg('expires_at'),
  sqlc.narg('link_provider_id'), sqlc.narg('link_subject'), sqlc.narg('link_proof')
);

-- name: FindPendingByHash :one
-- The second step's read: the pending row, its account, and the locale chain in one round trip,
-- FindSessionForAuth's shape.
SELECT p.id, p.account_id, p.purpose, p.user_agent, p.ip_class,
       p.created_at, p.expires_at, p.consumed_at, p.link_provider_id, p.link_subject, p.link_proof,
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

-- name: FindPendingByID :one
-- FindPendingByHash for a credential the server itself remembered rather than one a caller
-- presented: the CONNECT link a provider flow carries (ADR-0078 §1, SC-33). The flow kept the
-- credential's identifier, never its token. Row level security keeps it to the workspace the
-- transaction is bound to.
SELECT p.id, p.account_id, p.purpose, p.user_agent, p.ip_class,
       p.created_at, p.expires_at, p.consumed_at, p.link_provider_id, p.link_subject, p.link_proof,
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
WHERE p.id = sqlc.arg('id') AND a.deleted_at IS NULL;

-- name: ConsumePendingCredential :execrows
-- Single use, atomically: only the first completion writes, and a lost race answers exactly as
-- an unknown token does.
UPDATE auth_pending SET consumed_at = sqlc.arg('now')
WHERE id = sqlc.arg('id') AND consumed_at IS NULL;

-- name: SupersedePending :execrows
-- A new credential of a purpose replaces the account's earlier unspent ones (UC-ID-04 check 2: a
-- second reset request replaces the first link instead of adding one). Spent rather than deleted,
-- so a link that arrives late is refused as any spent one is.
UPDATE auth_pending SET consumed_at = sqlc.arg('now')
WHERE account_id = sqlc.arg('account_id') AND purpose = sqlc.arg('purpose') AND consumed_at IS NULL;

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

-- name: SealedMfaEnrollmentsNotUnder :many
-- The rows a re-seal visits (ADR-0045): every enrolment whose secret names a key other than the
-- current one. The key is a filter, never a tenant - row level security bounds this like every
-- other statement in the file.
SELECT account_id, secret_enc, secret_key_id, confirmed_at, last_step
FROM account_mfa
WHERE secret_key_id <> sqlc.arg('key_id')
ORDER BY account_id;

-- name: SealedMfaReplacementsNotUnder :many
-- A replacement's wrapping is a sealed value like the armed one's, so a rotation moves it too: the
-- census counts it, and a rotation that skipped it would report done while it named the leaving key.
SELECT account_id, replacement_secret_enc, replacement_secret_key_id
FROM account_mfa
WHERE replacement_secret_key_id IS NOT NULL AND replacement_secret_key_id <> sqlc.arg('key_id')
ORDER BY account_id;

-- name: RewrapMfaReplacement :execrows
-- RewrapMfaEnrollment's guard, for the replacement's wrapping.
UPDATE account_mfa
SET replacement_secret_enc = sqlc.arg('secret_enc'), replacement_secret_key_id = sqlc.arg('secret_key_id')
WHERE account_id = sqlc.arg('account_id') AND replacement_secret_key_id = sqlc.arg('expected_key_id');

-- name: RewrapMfaEnrollment :execrows
-- The wrapping moves and nothing else does: no updated_at the person would read as their
-- enrolment having changed. The guard on the key the row named is what keeps a re-seal from
-- overwriting an enrolment that was replaced between the read and this write.
UPDATE account_mfa
SET secret_enc = sqlc.arg('secret_enc'), secret_key_id = sqlc.arg('secret_key_id')
WHERE account_id = sqlc.arg('account_id') AND secret_key_id = sqlc.arg('expected_key_id');
