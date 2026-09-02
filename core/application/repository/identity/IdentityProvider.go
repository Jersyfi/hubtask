// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
)

// IdentityProviders is the workspace's configured provider (H-04).
//
// Reading the configuration and reading its secret are two methods, deliberately. The ordinary
// read is what a person and an auditor get, and it cannot spill the secret because the secret is
// not in what it answers; the exchange asks for the envelope by name, which makes opening it a
// decision somebody wrote down rather than a field that happened to be in a struct.
type IdentityProviders interface {
	// Upsert sets the configuration whole and answers it as stored. A workspace has one
	// provider, so this is a write with no identifier: the row's key is the workspace.
	Upsert(ctx context.Context, provider identity.IdentityProvider, sealed crypto.Sealed, now time.Time) (identity.IdentityProvider, error)

	// Find answers the configuration without its secret, or an error wrapping
	// shared.ErrNotFound when the workspace has configured none.
	Find(ctx context.Context) (identity.IdentityProvider, error)

	// FindWithSecret answers it with the sealed client secret, for the token exchange and for
	// nothing else.
	FindWithSecret(ctx context.Context) (identity.IdentityProvider, crypto.Sealed, error)

	// Delete removes the configuration and its sealed secret. False is "there was none", which
	// is not an error - a caller asking for it to be gone got what they asked for.
	Delete(ctx context.Context) (bool, error)
}

// OidcFlows keeps the handful of minutes between sending somebody to their provider and their
// coming back.
type OidcFlows interface {
	// Insert writes one flow. The presented state is hashed by the adapter, the way every other
	// presented token in this repository is.
	Insert(ctx context.Context, flow identity.OidcFlow, presented identity.Token) error

	// Consume judges and burns in one statement: unexpired, unconsumed, or nothing at all - so
	// a state presented twice is refused whoever races whom.
	Consume(ctx context.Context, presented identity.Token, now time.Time) (identity.OidcFlow, bool, error)
}

// ExternalAccounts is the seam phase 0 cut: `account.external_subject`, under the unique index
// that makes one provider subject one account per workspace.
type ExternalAccounts interface {
	// FindBySubject answers the account a provider's subject already names, or an error
	// wrapping shared.ErrNotFound on the first arrival.
	FindBySubject(ctx context.Context, subject string) (identity.Account, error)

	// LinkSubject writes the subject onto an account. False means the account was not there to
	// link; the unique index is what refuses a subject already spoken for, and it refuses rather
	// than this method, because two sign-ins racing must not both win.
	LinkSubject(ctx context.Context, accountID shared.ID, subject string, now time.Time) (bool, error)
}
