// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

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
