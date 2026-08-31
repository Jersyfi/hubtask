// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// AccountKind separates a person from a machine. The audit trail records it, because "the token
// of an automation deleted the collection" and "a colleague deleted the collection" are different
// events (ADR-0005, audit.md §2).
type AccountKind string

const (
	AccountUser           AccountKind = "USER"
	AccountServiceAccount AccountKind = "SERVICE_ACCOUNT"
)

// AccountStatus is the lifecycle of an account.
type AccountStatus string

const (
	AccountActive   AccountStatus = "ACTIVE"
	AccountInvited  AccountStatus = "INVITED"
	AccountDisabled AccountStatus = "DISABLED"
	// AccountRestricted is Art. 18 as a technical state (data-protection.md §4, E-10): the person
	// still works, and what stops is the *processing* of their data - no automatic decision is
	// made about them, and no AI is shown their content.
	//
	// It is deliberately not a lockout. Restricting somebody's processing and disabling their
	// account are two different answers to two different situations, and giving the first the
	// effect of the second would punish a person for exercising a right.
	AccountRestricted AccountStatus = "RESTRICTED"
	// AccountAnonymized is the end of a life: an erasure carried out in the mode that keeps the
	// authorship, so that the workspace's own content stays readable and everything of the
	// person's in it is gone. Such an account cannot act - there is nobody left to act.
	AccountAnonymized AccountStatus = "ANONYMIZED"
)

// MayAct reports whether an account in this state may make requests at all.
func (s AccountStatus) MayAct() bool { return s == AccountActive || s == AccountRestricted }

// ProcessingAllowed reports whether this system may make automatic decisions about the person or
// pass their content to anything that processes it (Art. 18).
//
// The one predicate every such place asks, rather than a status comparison written out wherever
// automatic processing happens: the rule engine and the AI features arrive later, and what they
// have to consult should already exist and already be the same question.
func (s AccountStatus) ProcessingAllowed() bool {
	return s != AccountRestricted && s != AccountAnonymized
}

// MaxTokenLifetime is how far into the future an expiry may be set (security.md §5). A year is
// long enough that nobody works around it and short enough that a forgotten credential stops
// working while the person who made it is still around to be asked about it.
const MaxTokenLifetime = 365 * 24 * time.Hour

// MaxTokenNameLength matches the column and the contract.
const MaxTokenNameLength = 200

// AccessToken is the stored half of a personal access token. The secret itself is not here and
// never was: only its hash is stored, and the hash never leaves the persistence adapter
// (security.md §8).
type AccessToken struct {
	ID        shared.ID
	TenantID  shared.ID
	AccountID shared.ID
	// Name is what the credential is for, in its owner's words. It is what they read in a year
	// when deciding whether it is still needed, which is the only thing that makes an old token
	// revocable rather than merely revoked-by-guessing.
	Name string
	// Scopes bound what the token may do, independently of the role its owner holds.
	Scopes []string
	// ExpiresAt is mandatory. A token without an end is a credential nobody ever revokes, so the
	// zero value counts as expired rather than as "no expiry" (security.md §5).
	ExpiresAt time.Time
	// RevokedAt is set when the token was withdrawn; zero means it was not.
	RevokedAt time.Time
	// LastUsedAt is what makes an unused token visible to its owner. It is written back at most
	// once per interval, because the alternative is a write on every request.
	LastUsedAt time.Time
	// CreatedAt is when it was minted, and the order a listing reads in.
	CreatedAt time.Time
}

// NewAccessTokenInput is what minting one needs. The secret is not among the fields: the domain
// draws no randomness (rule 4), and the plaintext never belongs to the stored half at all.
type NewAccessTokenInput struct {
	ID        shared.ID
	TenantID  shared.ID
	AccountID shared.ID
	Name      string
	Scopes    []string
	ExpiresAt time.Time
	Now       time.Time
}

// NewAccessToken builds the row a mint will store, and refuses anything security.md §5 forbids.
//
// Whether the scopes exist is deliberately not asked here. The set of scopes an installation
// declares is the use case catalogue's, and the domain does not know there is a catalogue - the
// application layer checks the names against it and this checks their shape (ADR-0001).
func NewAccessToken(in NewAccessTokenInput) (AccessToken, error) {
	name := strings.TrimSpace(in.Name)
	switch {
	case in.ID.IsZero() || in.TenantID.IsZero() || in.AccountID.IsZero():
		return AccessToken{}, shared.ErrInternal.WithDetail("access.token_incomplete")
	case name == "":
		return AccessToken{}, fieldError("/name", "access.token_name_required")
	case len(name) > MaxTokenNameLength:
		return AccessToken{}, fieldError("/name", "access.token_name_too_long")
	}

	scopes, err := normaliseScopes(in.Scopes)
	if err != nil {
		return AccessToken{}, err
	}
	if err := checkExpiry(in.ExpiresAt, in.Now); err != nil {
		return AccessToken{}, err
	}

	return AccessToken{
		ID: in.ID, TenantID: in.TenantID, AccountID: in.AccountID,
		Name: name, Scopes: scopes,
		ExpiresAt: in.ExpiresAt.UTC(), CreatedAt: in.Now.UTC(),
	}, nil
}

// Revoked stamps the token. Idempotent in the caller's sense: a token already revoked keeps the
// moment it was, because the first withdrawal is the one that mattered.
func (t AccessToken) Revoked(at time.Time) AccessToken {
	if !t.RevokedAt.IsZero() {
		return t
	}
	t.RevokedAt = at.UTC()
	return t
}

// IsRevoked reports whether somebody pulled it, which is a different question from whether it ran
// out - and the reason the two are separate columns.
func (t AccessToken) IsRevoked() bool { return !t.RevokedAt.IsZero() }

// checkExpiry holds the two halves of "mandatory, and at most a year".
//
// There is no default. A caller has to choose, because every default that could be written here
// is either so short that people work around it or so long that it is the eternal credential the
// rule exists to prevent.
func checkExpiry(expiresAt, now time.Time) error {
	if expiresAt.IsZero() {
		return fieldError("/expires_at", "access.token_expiry_required")
	}
	if !expiresAt.After(now) {
		return fieldError("/expires_at", "access.token_expiry_past")
	}
	if expiresAt.After(now.Add(MaxTokenLifetime)) {
		return shared.ErrValidation.
			WithDetail("access.token_expiry_too_far").
			WithParams(map[string]string{"max_days": "365"}).
			WithFields(shared.FieldError{Path: "/expires_at", Code: "access.token_expiry_too_far"})
	}
	return nil
}

// normaliseScopes refuses an empty request and collapses repeats.
//
// Empty is refused rather than read as "everything": a token that asks for nothing and receives
// everything is the failure mode the explicit request exists to rule out, and a caller that meant
// to ask for nothing has nothing to use the token for.
func normaliseScopes(requested []string) ([]string, error) {
	scopes := make([]string, 0, len(requested))
	seen := make(map[string]bool, len(requested))
	for _, scope := range requested {
		scope = strings.TrimSpace(scope)
		if scope == "" || seen[scope] {
			continue
		}
		seen[scope] = true
		scopes = append(scopes, scope)
	}
	if len(scopes) == 0 {
		return nil, fieldError("/scopes", "access.token_scopes_required")
	}
	// Sorted, so that two tokens asking for the same rights are stored identically and a listing
	// reads the same way twice.
	slices.Sort(scopes)
	return scopes, nil
}

func fieldError(path, code string) error {
	return shared.ErrValidation.
		WithDetail(code).
		WithFields(shared.FieldError{Path: path, Code: code})
}

// Verify decides whether the token may still be used at this moment.
//
// The order is deliberate: revocation before expiry, because a revoked token is a security event
// and an expired one is routine, and whoever reads the log should see the first of those.
func (t AccessToken) Verify(now time.Time) error {
	if !t.RevokedAt.IsZero() && !now.Before(t.RevokedAt) {
		return shared.ErrUnauthenticated.WithDetail("access.token_revoked")
	}
	if t.ExpiresAt.IsZero() || !now.Before(t.ExpiresAt) {
		// A missing expiry is refused rather than treated as eternal. The column allows NULL, the
		// contract does not - and of the two readings, "refuse it" is the one that cannot leave a
		// forgotten credential valid for ever.
		return shared.ErrUnauthenticated.WithDetail("access.token_expired")
	}
	return nil
}

// NeedsTouch reports whether the last use is stale enough to be worth a write. Any request
// refreshes it at most once per interval: the value exists so an owner can spot a token nobody
// uses, and for that a resolution of minutes is plenty.
func (t AccessToken) NeedsTouch(now time.Time, interval time.Duration) bool {
	return t.LastUsedAt.IsZero() || now.Sub(t.LastUsedAt) >= interval
}

// Account is as much of the acting account as authentication needs: whether it may act at all,
// what kind of actor it is, and how it prefers to be spoken to.
type Account struct {
	ID shared.ID
	// TenantID is the workspace the account belongs to. An account exists in exactly one: two
	// workspaces for one person are two accounts, which is what keeps a membership from ever
	// having to name a tenant (multi-tenancy.md §2).
	TenantID shared.ID
	Kind     AccountKind
	// Email is how a person is invited and how they are found again. Unique per tenant, and
	// stored lower case because two spellings of one address are two accounts for one person.
	Email string
	// DisplayName is what the audit trail records alongside the identifier. Denormalised into
	// every entry it appears in, because an entry that only points at a foreign key becomes
	// unreadable once the account is deleted, and a trail that loses its meaning through a
	// deletion does not do its job (audit.md §2, test AT-7).
	DisplayName string
	Status      AccountStatus
	// Locale, TimeZone and WeekStart are the account's own preferences, the second link of the
	// resolution chain request → account → tenant → installation (i18n-l10n.md §2). Any of them
	// may be empty, which means the tenant's default applies.
	Locale    string
	TimeZone  string
	WeekStart string
}

// Verify decides whether the account may act.
//
// An invited account counts as unable: the invitation has not been accepted, so nobody has proven
// they own it. Refusing is 403 rather than 401 - the credential was valid, the account is not,
// and asking the client to authenticate again would send it round a loop it cannot leave.
//
// A **restricted** account may act. Art. 18 restricts what the controller does with the person's
// data, not what the person may do; treating a restriction as a lockout would punish somebody for
// exercising a right (E-10). An anonymised one may not: there is nobody left to act.
func (a Account) Verify() error {
	if !a.Status.MayAct() {
		return shared.ErrForbidden.
			WithDetail("access.account_not_active").
			WithParams(map[string]string{"status": string(a.Status)})
	}
	return nil
}
