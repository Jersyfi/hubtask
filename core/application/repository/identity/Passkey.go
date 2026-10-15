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
