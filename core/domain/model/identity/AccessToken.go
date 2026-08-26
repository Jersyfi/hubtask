// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
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

// AccessToken is the stored half of a personal access token. The secret itself is not here and
// never was: only its hash is stored, and the hash never leaves the persistence adapter
// (security.md §8).
type AccessToken struct {
	ID        shared.ID
	TenantID  shared.ID
	AccountID shared.ID
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
