// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// PasskeyCredentials is the WebAuthn credentials of the transaction's workspace (identity.md §23).
//
// The limit of ten is the use case's to keep, and it keeps it with LockAccount: the count and the
// insert run after the account's row is locked, so two registrations finishing at once cannot both
// read nine (known traps: a read-then-write without a lock).
type PasskeyCredentials interface {
	// ListOf answers the account's credentials, oldest first.
	ListOf(ctx context.Context, accountID shared.ID) ([]identity.PasskeyCredential, error)

	// CountOf answers how many the account holds.
	CountOf(ctx context.Context, accountID shared.ID) (int, error)

	// LockAccount locks the account's row until the transaction ends; ErrNotFound where there is
	// none in this workspace.
	LockAccount(ctx context.Context, accountID shared.ID) error

	// Insert keeps a credential, and answers false where the workspace already holds one with its
	// identifier - nothing is written then.
	Insert(ctx context.Context, credential identity.PasskeyCredential) (bool, error)

	// Rename changes the name of one of the account's credentials and answers it; ErrNotFound where
	// the account holds none by that id.
	Rename(ctx context.Context, accountID, id shared.ID, name string) (identity.PasskeyCredential, error)

	// Remove deletes one of the account's credentials; false where the account holds none by that id.
	Remove(ctx context.Context, accountID, id shared.ID) (bool, error)
}

// PasskeyChallenges is a registration's challenge, waiting in auth_pending under its own purpose
// (identity.md §23.3, migration 0161). The token is the challenge under its own prefix, so only its
// hash is stored.
type PasskeyChallenges interface {
	// Issue supersedes the account's unspent challenges and writes this one: the newest start wins,
	// so two starts never leave two registrations to finish.
	Issue(ctx context.Context, challenge identity.PasskeyChallenge, presented identity.Token) error

	// Consume spends the challenge in one statement - only for its purpose, its session and its
	// account, unspent and in time - and answers it; false where nothing matched, which is every
	// stale, foreign or unknown challenge alike.
	Consume(
		ctx context.Context, presented identity.Token, sessionID, accountID shared.ID, now time.Time,
	) (identity.PasskeyChallenge, bool, error)
}

// PasskeyHolder is a credential a browser named, with its holder as a sign-in needs them.
type PasskeyHolder struct {
	Credential   identity.PasskeyCredential
	Account      identity.Account
	TenantStatus identity.TenantStatus
	// StartPasswordAt is when Hubtask drew the password the account holds; zero for one the person
	// chose. A start password owes the change at the next sign-in, by any door (identity.md §13).
	StartPasswordAt time.Time
}

// PasskeySignIns is what signing in with a passkey reads and writes (identity.md §23.4): the
// challenge that names nobody, the credential the browser names, and its counter.
type PasskeySignIns interface {
	// IssueChallenge writes a sign-in's challenge with no account.
	IssueChallenge(ctx context.Context, challenge identity.PendingCredential, presented identity.Token) error

	// SpendChallenge spends it in one statement - only under its purpose, unspent and in time - and
	// answers false where nothing matched.
	SpendChallenge(ctx context.Context, presented identity.Token, now time.Time) (bool, error)

	// HolderOf answers the credential with this identifier in the transaction's workspace and its
	// holder, or ErrNotFound.
	HolderOf(ctx context.Context, credentialID []byte) (PasskeyHolder, error)

	// MarkUsed records a verified sign-in: the counter, never lowered, the moment and the backup
	// state - and answers false, writing nothing, where the counter is not above a non-zero stored
	// one.
	MarkUsed(ctx context.Context, id shared.ID, signCount uint32, backedUp bool, at time.Time) (bool, error)
}
