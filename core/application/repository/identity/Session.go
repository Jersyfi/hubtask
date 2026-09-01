// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// SessionCredential is what authenticating a session access token yields: the session the
// signature named, the account behind it, and the tenant's defaults - one round trip, for
// Credential's reason.
type SessionCredential struct {
	Session identity.Session
	Account identity.Account
	// ClientID is the OAuth client behind a grant session (H-05), zero for a person's own.
	ClientID shared.ID
	// TenantLocale and TenantTimeZone are the third link of the resolution chain.
	TenantLocale   string
	TenantTimeZone string
	// TenantSlug and TenantStatus ride with every credential read, Credential's reason (H-06).
	TenantSlug   string
	TenantStatus identity.TenantStatus
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
	// TenantSlug and TenantStatus ride with every credential read, Credential's reason (H-06).
	TenantSlug   string
	TenantStatus identity.TenantStatus
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
	// TenantSlug and TenantStatus ride with every credential read, Credential's reason (H-06).
	TenantSlug   string
	TenantStatus identity.TenantStatus
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
	// TenantSlug and TenantStatus ride with every credential read, Credential's reason (H-06).
	TenantSlug   string
	TenantStatus identity.TenantStatus
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

	// PasswordHashOf answers one account's stored hash, for the operations that demand the
	// password afresh of somebody already signed in - disabling the second factor is the first
	// (H-02, security.md §5). Empty for an account that signs in some other way.
	PasswordHashOf(ctx context.Context, accountID shared.ID) (secret.Secret, error)
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

// MfaEnrollment is the stored second factor (H-02): the sealed secret and what arms it. The
// secret travels sealed - opening it is the application layer's act, through the Encryptor,
// because verification needs the plaintext and storage never does.
type MfaEnrollment struct {
	AccountID shared.ID
	Secret    crypto.Sealed
	// ConfirmedAt is what arms it; zero means enrolment began and protects nobody yet.
	ConfirmedAt time.Time
	// LastStep is the highest accepted RFC 6238 step - the replay refusal's floor.
	LastStep int64
}

// MfaEnrollments maintains the one enrolment an account can hold.
type MfaEnrollments interface {
	// Upsert writes a fresh enrolment or replaces an unconfirmed one. False means an armed
	// enrolment stands - "disable first, with the password".
	Upsert(ctx context.Context, accountID shared.ID, sealed crypto.Sealed, now time.Time) (bool, error)

	// Find answers the enrolment, or an error wrapping shared.ErrNotFound.
	Find(ctx context.Context, accountID shared.ID) (MfaEnrollment, error)

	// Confirm arms an unconfirmed enrolment and records the confirming step in the same
	// statement. False means there was nothing unconfirmed to arm.
	Confirm(ctx context.Context, accountID shared.ID, step int64, now time.Time) (bool, error)

	// RecordStep advances the replay floor, atomically: false means the step was not past it -
	// the same or an older code presented again.
	RecordStep(ctx context.Context, accountID shared.ID, step int64, now time.Time) (bool, error)

	// Disable removes the enrolment whole. False means there was none.
	Disable(ctx context.Context, accountID shared.ID) (bool, error)
}

// RecoveryCodes maintains the factor's escape hatch. Presented codes travel whole and are hashed
// in the adapter, the pepper's home.
type RecoveryCodes interface {
	// Replace burns whatever stood and stores the new set's hashes.
	Replace(ctx context.Context, accountID shared.ID, ids []shared.ID, presented []string, now time.Time) error

	// Burn consumes one code. False means it matched nothing live - wrong, already used, or
	// somebody else's, indistinguishably.
	Burn(ctx context.Context, accountID shared.ID, presented string, now time.Time) (bool, error)

	// Remaining counts what is left, for the answer that tells a person to re-enrol in time.
	Remaining(ctx context.Context, accountID shared.ID) (int, error)
}

// PendingLookup is what presenting a pending credential yields: the row, its account, and the
// locale chain - one round trip, SessionCredential's reason.
type PendingLookup struct {
	Credential identity.PendingCredential
	Account    identity.Account
	// TenantLocale and TenantTimeZone are the third link of the resolution chain.
	TenantLocale   string
	TenantTimeZone string
	// TenantSlug and TenantStatus ride with every credential read, Credential's reason (H-06).
	TenantSlug   string
	TenantStatus identity.TenantStatus
}

// PendingCredentials maintains the two-step sign-in's middle state.
type PendingCredentials interface {
	// Insert writes the row the password answered.
	Insert(ctx context.Context, credential identity.PendingCredential, presented identity.Token) error

	// FindByToken answers what a presented pending token names, or an error wrapping
	// shared.ErrNotFound.
	FindByToken(ctx context.Context, token identity.Token) (PendingLookup, error)

	// Consume marks the credential used, atomically: false means somebody was here first.
	Consume(ctx context.Context, credentialID shared.ID, at time.Time) (bool, error)
}

// TenantPolicy answers the tenant's security switches (H-02). A narrow reader rather than the
// settings document, so the application layer never parses a shape the adapter owns.
type TenantPolicy interface {
	// RequireAdminTotp reports whether this tenant demands TOTP of OWNER and ADMIN role holders
	// (security.md §5). An absent switch is false: enforcement is a decision, never a default.
	RequireAdminTotp(ctx context.Context) (bool, error)
}

// StepUps maintains the proof a privileged action demands (H-03). The presented token travels
// whole and is hashed in the adapter, the pepper's home.
type StepUps interface {
	// Record lands a fresh proof on the caller's own live session, replacing whatever stood.
	// False means the session is not the account's, or not live.
	Record(
		ctx context.Context, sessionID, accountID shared.ID,
		presented identity.Token, method identity.StepUpMethod, at time.Time,
	) (bool, error)

	// Consume judges and burns the proof in one statement: fresh within the cutoff, unconsumed,
	// on a live session of this account - or false, whatever the reason. The method that proved
	// it comes back for the trail.
	Consume(
		ctx context.Context, presented identity.Token, accountID shared.ID,
		cutoff, now time.Time,
	) (identity.StepUpMethod, bool, error)
}

// GrantListing is one row of the grants a person sees: the grant, its client's name, and when a
// session under it last acted - computed, not written back.
type GrantListing struct {
	Grant      identity.OauthGrant
	ClientName string
	LastUsedAt time.Time
}

// OauthClients is the registry of third-party apps (H-05). The presented secret travels whole
// and is hashed in the adapter, the pepper's home.
type OauthClients interface {
	// Insert writes a registration. The presented secret is the zero token for a public client.
	Insert(ctx context.Context, client identity.OauthClient, secret identity.Token) error

	// List answers the registry, newest first.
	List(ctx context.Context) ([]identity.OauthClient, error)

	// Find answers one client, or an error wrapping shared.ErrNotFound.
	Find(ctx context.Context, clientID shared.ID) (identity.OauthClient, error)

	// SecretMatches compares a presented secret against the stored hash, in the adapter. False
	// for a wrong secret and for a public client alike - which of the two is not for the
	// presenter to learn.
	SecretMatches(ctx context.Context, clientID shared.ID, presented secret.Secret) (bool, error)

	// Delete removes the registration; the grants and their sessions go by cascade.
	Delete(ctx context.Context, clientID shared.ID) (bool, error)
}

// OauthGrants maintains what people allowed.
type OauthGrants interface {
	// Upsert records a consent: one live grant per person and app, the fresh scopes replacing
	// the old. The identifier of the live grant comes back.
	Upsert(ctx context.Context, grant identity.OauthGrant) (shared.ID, error)

	// Find answers one grant, or an error wrapping shared.ErrNotFound.
	Find(ctx context.Context, grantID shared.ID) (identity.OauthGrant, error)

	// ListForAccount answers the account's live grants, newest first, with the client's name.
	ListForAccount(ctx context.Context, accountID shared.ID) ([]GrantListing, error)

	// Revoke withdraws one of the account's grants; false is the indistinguishable not-found.
	Revoke(ctx context.Context, grantID, accountID shared.ID, at time.Time) (bool, error)

	// RevokeSessions ends every session the grant leashed, immediately.
	RevokeSessions(ctx context.Context, grantID shared.ID, at time.Time) (int, error)
}

// OauthCodes maintains the single-use authorization codes.
type OauthCodes interface {
	// Insert writes a minted code beside its challenge and redirect.
	Insert(ctx context.Context, code identity.OauthCode, presented identity.Token) error

	// Consume judges and burns in one statement: unexpired, unconsumed, or nothing.
	Consume(ctx context.Context, presented identity.Token, now time.Time) (identity.OauthCode, bool, error)
}
