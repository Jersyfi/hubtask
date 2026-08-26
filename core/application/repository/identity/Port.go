// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package identity declares what the identity use cases need from storage. The interfaces live
// here, with their callers; the implementations live in infrastructure/postgres
// (project-structure.md §2).
package identity

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Credential is what one lookup yields: the token, the account behind it, and the tenant's
// defaults.
//
// All three in one result rather than three calls, because this runs on every single request. The
// tenant's preferences are two columns on a row the query has to reach anyway, and the difference
// between one round trip and three is the difference between an API that feels immediate and one
// that does not.
type Credential struct {
	Token   identity.AccessToken
	Account identity.Account
	// TenantLocale and TenantTimeZone are the third link of the resolution chain
	// (i18n-l10n.md §2). Never empty: the columns have defaults.
	TenantLocale   string
	TenantTimeZone string
}

// AccessTokens finds and maintains personal access tokens.
//
// The presented token is passed whole rather than pre-hashed. Hashing needs the server-side
// pepper, and the pepper has no business in the application layer - it is a secret of the
// persistence adapter, which is also the only place that can compare against what is stored
// (security.md §8).
type AccessTokens interface {
	// FindByToken returns the credential a presented token names, or an error wrapping
	// shared.ErrNotFound when the hash matches nothing.
	//
	// It reports what is stored and judges none of it: expiry, revocation and account status are
	// decided by the use case, so the rules stay where they can be tested without a database
	// (ADR-0001).
	FindByToken(ctx context.Context, token identity.Token) (Credential, error)

	// TouchLastUsed records that the token was used. Called at most once per interval, so an
	// owner can see a token nobody uses without every request costing a write.
	TouchLastUsed(ctx context.Context, tokenID shared.ID, at time.Time) error
}

// Memberships answers what an account holds.
type Memberships interface {
	// Along returns the memberships that apply anywhere on the path, held by the account itself
	// or through one of its groups.
	//
	// The path is passed in rather than the whole set being read, because an account in a large
	// tenant may hold hundreds of memberships and a permission check runs on every write. The
	// query may be generous - the resolution ignores what is not on the path - but it must not be
	// unbounded.
	//
	// A group membership comes back as a membership of the account. Whether a right is held
	// directly or through a group is not a distinction the decision makes, and resolving it in
	// the query costs a join that the index already serves.
	Along(ctx context.Context, accountID shared.ID, path []identity.Scope) ([]identity.Membership, error)

	// SharedItemsIn returns the entries inside one collection the account holds a membership on -
	// what was shared with it individually (domain-model.md §3.2, C-04).
	//
	// Along cannot answer this: it takes the path to one entry, and this asks which entries there
	// are a path to. The two are asked in sequence and only when the first came back empty - an
	// account that holds a role on the collection reads all of it, and never pays for this query.
	//
	// Bounded to one collection rather than the tenant, because the caller asked about one
	// level. Trashed and archived entries are in the answer as they are stored: which of them the
	// level shows is the item query's rule, and applying it twice is how the two come to disagree.
	SharedItemsIn(ctx context.Context, accountID, collectionID shared.ID) ([]shared.ID, error)
}

// Accounts is the store of people and service accounts.
//
// The write side is small on purpose: 0.2.0 invites an account and changes its preferences.
// Accepting an invitation, changing an email address and disabling an account belong to the
// sign-in flow and arrive with it (security.md §5).
type Accounts interface {
	// Find returns the account, or an error wrapping shared.ErrNotFound. The tenant is the
	// transaction's; an account of another tenant is not found rather than forbidden, because
	// anything else confirms that it exists (multi-tenancy.md §2).
	Find(ctx context.Context, accountID shared.ID) (identity.Account, error)

	// FindByEmail is how an invitation notices that the person is already here. The address is
	// compared the way it is stored, lower case.
	FindByEmail(ctx context.Context, email string) (identity.Account, error)

	// Insert writes a new account. It fails with a conflict when the address is taken, because
	// the uniqueness is the database's to enforce and racing callers must not both win.
	Insert(ctx context.Context, account identity.Account) error

	// UpdatePreferences writes the three preference columns and nothing else. A method per
	// concern rather than a general update: an update that can write any column is one that can
	// write the status by accident.
	UpdatePreferences(ctx context.Context, account identity.Account, at time.Time) error

	// Restricted answers which of the accounts named may not be processed automatically - Art. 18
	// as a technical state (data-protection.md §4, E-10).
	//
	// A set rather than a question per account, because the caller is a draw over a pool: one
	// round trip for a policy's candidates instead of one per candidate. Accounts that are not
	// restricted are simply absent from the answer.
	Restricted(ctx context.Context, accountIDs []shared.ID) (map[shared.ID]bool, error)
}

// Groups is the store of named sets of accounts.
type Groups interface {
	Find(ctx context.Context, groupID shared.ID) (identity.Group, error)

	// Insert writes a new group, and conflicts when the name is taken within the tenant.
	Insert(ctx context.Context, group identity.Group) error

	// Update writes the name and description under optimistic locking: it takes the version the
	// caller read and fails with a version conflict when the row has moved on since.
	Update(ctx context.Context, group identity.Group, expectedVersion int) error

	// Delete removes the group. Its memberships and its member links go with it, which is the
	// database's cascade rather than three statements that could half-run.
	Delete(ctx context.Context, groupID shared.ID) error

	// AddMember and RemoveMember maintain who is in the group. Both are idempotent: adding
	// somebody twice is what a retry looks like, and it is not an error.
	AddMember(ctx context.Context, groupID shared.ID, accountID shared.ID) error
	RemoveMember(ctx context.Context, groupID shared.ID, accountID shared.ID) error

	// Members is who is in it. Read only where a replacement needs to know what to remove - a
	// permission check never asks, because a group membership reaches it as a membership of the
	// account (see Memberships.Along).
	Members(ctx context.Context, groupID shared.ID) ([]shared.ID, error)
}

// MembershipGrants is the write half of Memberships. Separate interface, same table: reading is
// on the hot path of every request and writing happens when an administrator acts, and a use case
// that only grants should not be handed the ability to resolve.
type MembershipGrants interface {
	// Grant records the membership, or does nothing when the same grant already exists. The
	// identifier is the caller's, so that the audit entry and the row agree.
	//
	// A grant names an account or a group, never both: that is the database's constraint, and
	// the use case checks it before it gets there.
	Grant(ctx context.Context, grant identity.Grant) error

	// Revoke removes it and reports whether anything was there. The distinction is what lets the
	// use case answer "not found" rather than pretending it removed something.
	Revoke(ctx context.Context, membershipID shared.ID) (bool, error)

	// Find returns one membership, for the use case that has to know what it is about to revoke -
	// the audit entry names the scope and the role, and a trail that only records an identifier
	// is unreadable a year later (audit.md §2).
	Find(ctx context.Context, membershipID shared.ID) (identity.Grant, error)
}
