// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// SessionCredential is what authenticating a session access token yields: the session the
// signature named, the account behind it, and the tenant's defaults - one round trip, for
// Credential's reason.
type SessionCredential struct {
	Session identity.Session
	Account identity.Account
	// TenantLocale and TenantTimeZone are the third link of the resolution chain.
	TenantLocale   string
	TenantTimeZone string
}

// RefreshCredential is what the exchange reads: the presented link of the chain, its session,
// the account, and the tenant's defaults.
type RefreshCredential struct {
	Token   identity.RefreshToken
	Session identity.Session
	Account identity.Account
	// TenantLocale and TenantTimeZone are the third link of the resolution chain.
	TenantLocale   string
	TenantTimeZone string
}

// SignInAccount is what the credential check reads. The stored hash travels as a secret: the
// application layer holds it only long enough to hand it to the verifier, and no way of printing
// it yields anything (rule 10).
type SignInAccount struct {
	Account identity.Account
	// PasswordHash is empty for an account that signs in some other way - a service account, or
	// later an OIDC account. The check then runs against the decoy, so the refusal costs the
	// same work either way (T-02).
	PasswordHash secret.Secret
	// TenantLocale and TenantTimeZone are the third link of the resolution chain.
	TenantLocale   string
	TenantTimeZone string
}

// RedemptionAccount is what redeeming an invitation reads.
type RedemptionAccount struct {
	Account identity.Account
	// ExpiresAt is when the token stops being redeemable. Judged by the use case, stated by the
	// row.
	ExpiresAt time.Time
	// TenantLocale and TenantTimeZone are the third link of the resolution chain.
	TenantLocale   string
	TenantTimeZone string
}

// AuthAttempt is one subject's standing in the attempt ledger (T-02).
type AuthAttempt struct {
	Failures      int
	LastFailureAt time.Time
	LockedUntil   time.Time
}

// Sessions finds and maintains the sign-in rows. No method takes a tenant: row level security
// bounds every statement to the tenant of the running transaction (ADR-0010).
type Sessions interface {
	// Insert writes the row a sign-in opens.
	Insert(ctx context.Context, session identity.Session) error

	// FindForAuth returns what a verified access token's claims point at, or an error wrapping
	// shared.ErrNotFound. It reports what is stored and judges none of it (ADR-0001).
	FindForAuth(ctx context.Context, sessionID shared.ID) (SessionCredential, error)

	// ForAccount answers the account's live sessions, newest first. The dead ones are absent: a
	// listing is for deciding what to end.
	ForAccount(ctx context.Context, accountID shared.ID, now time.Time) ([]identity.Session, error)

	// TouchLastSeen records that the session acted. Called at most once per interval.
	TouchLastSeen(ctx context.Context, sessionID shared.ID, at time.Time) error

	// Extend slides the session's horizon to the newest refresh token's.
	Extend(ctx context.Context, sessionID shared.ID, expiresAt time.Time) error

	// Revoke stamps one of the account's sessions and reports whether anything changed. Nothing
	// changed means it was somebody else's, unknown, or already ended - which of those is not
	// the caller's to learn (the indistinguishable not-found).
	Revoke(ctx context.Context, sessionID, accountID shared.ID, at time.Time) (bool, error)

	// RevokeAll ends every live session of the account and reports how many.
	RevokeAll(ctx context.Context, accountID shared.ID, at time.Time) (int, error)
}

// RefreshTokens maintains the rotating chain. The presented token is passed whole rather than
// pre-hashed, for AccessTokens' reason: the pepper is the adapter's secret.
type RefreshTokens interface {
	// Insert writes a freshly minted link of a session's chain.
	Insert(ctx context.Context, token identity.RefreshToken, presented identity.Token) error

	// FindByToken returns what a presented refresh token names - rotated links included, because
	// a rotated hash presented again is the reuse signal - or an error wrapping
	// shared.ErrNotFound.
	FindByToken(ctx context.Context, token identity.Token) (RefreshCredential, error)

	// Rotate retires the link and reports whether this call was the one that did. False means
	// somebody was here first, which on this path is the reuse alarm.
	Rotate(ctx context.Context, tokenID shared.ID, at time.Time) (bool, error)
}

// SignInAccounts is the account surface of the sign-in flow: the credential check's read and the
// invitation redeemed.
type SignInAccounts interface {
	// FindForSignIn returns the account an address names together with its stored hash, or an
	// error wrapping shared.ErrNotFound. The caller turns both refusals into one answer (T-02).
	FindForSignIn(ctx context.Context, email string) (SignInAccount, error)

	// SetRedemptionToken stores the hash of a freshly minted redemption token on an invited
	// account, replacing any earlier one. False means the account is not waiting.
	SetRedemptionToken(
		ctx context.Context, accountID shared.ID, presented identity.Token,
		expiresAt, now time.Time,
	) (bool, error)

	// FindByRedemptionToken returns the account a presented redemption token names, or an error
	// wrapping shared.ErrNotFound.
	FindByRedemptionToken(ctx context.Context, token identity.Token) (RedemptionAccount, error)

	// Redeem sets the first password, activates the account, and kills the token, in one
	// statement so no half of it can happen alone. False means the account was not waiting any
	// more - a second redemption, refused by the same indistinguishable answer as an unknown
	// token.
	Redeem(ctx context.Context, accountID shared.ID, passwordHash string, now time.Time) (bool, error)
}

// AuthAttempts is the attempt ledger (T-02). Subjects travel in clear and are stored only as
// hashes under the ledger's own purpose label - the hashing is the adapter's, for the pepper's
// reason.
type AuthAttempts interface {
	// Find answers a subject's standing. A subject never seen is the zero value, not an error.
	Find(ctx context.Context, subject string) (AuthAttempt, error)

	// Record writes a subject's standing after a failure.
	Record(ctx context.Context, subject string, attempt AuthAttempt) error

	// Clear wipes a subject's slate after a success.
	Clear(ctx context.Context, subject string) error
}

// TenantDirectory answers decision 3's question: which tenant is signing in, before any
// credential exists to say so. One identifier or none, never a listing - the implementation is
// the narrow SECURITY DEFINER path migration 0063 pins down.
type TenantDirectory interface {
	// Resolve maps a slug to its tenant. The empty slug answers the single-mode installation's
	// only row. No match is an error wrapping shared.ErrNotFound.
	Resolve(ctx context.Context, slug string) (shared.ID, error)
}
