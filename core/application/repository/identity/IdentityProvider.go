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

// IdentityProviders is the providers in force where the transaction is (H-04, SI-10).
//
// No method takes a tenant, and one of them deliberately sees two levels: the read policy admits a
// workspace's own rows and the installation's together (migration 0103), so `List` is the answer to
// "how can somebody sign in here" without a caller having to ask twice and merge.
//
// Reading the configuration and reading its secret are two methods, deliberately. The ordinary
// read is what a person and an auditor get, and it cannot spill the secret because the secret is
// not in what it answers; the exchange asks for the envelope by name, which makes opening it a
// decision somebody wrote down rather than a field that happened to be in a struct.
type IdentityProviders interface {
	// List answers the providers in force here - the workspace's own first, then the
	// installation's - in the order their buttons are drawn.
	List(ctx context.Context) ([]identity.IdentityProvider, error)

	// Count answers how many this level has configured, for the bound an insert is held to. The
	// installation's rows are not a workspace's to be limited by, so they are not in it.
	Count(ctx context.Context) (int, error)

	// Find answers one by its identifier without its secret, or an error wrapping
	// shared.ErrNotFound. A workspace finds the installation's rows too: it has to, because one
	// of them may be the way its own people sign in.
	Find(ctx context.Context, id shared.ID) (identity.IdentityProvider, error)

	// FindWithSecret answers it with the sealed client secret, for the token exchange and for
	// nothing else.
	FindWithSecret(ctx context.Context, id shared.ID) (identity.IdentityProvider, crypto.Sealed, error)

	// Insert writes a new one. Which level it lands at is the scope's answer and not a
	// parameter's: the statement writes `current_tenant_id()`, which is NULL in the
	// installation's own scope.
	Insert(ctx context.Context, provider identity.IdentityProvider, sealed crypto.Sealed) (identity.IdentityProvider, error)

	// Update sets one whole. A nil `sealed` is "keep the secret that is already there", which is
	// the only way to change a display name without retyping a value nothing can read back.
	// False is "no such row here", which includes a workspace reaching for the installation's.
	Update(ctx context.Context, provider identity.IdentityProvider, sealed *crypto.Sealed, now time.Time) (identity.IdentityProvider, bool, error)

	// Reconfigure is Update without the switch: every field but `enabled`, which stays exactly as
	// the row holds it, in the same statement (ADR-0076 §5). A workspace's own form configures and
	// never switches, so its write does not touch the column the list of ways to sign in writes -
	// and a save cannot undo a switch made while the form was open.
	Reconfigure(ctx context.Context, provider identity.IdentityProvider, sealed *crypto.Sealed, now time.Time) (identity.IdentityProvider, bool, error)

	// Delete removes one and its sealed secret. False is "there was none", which is not an error -
	// a caller asking for it to be gone got what they asked for.
	Delete(ctx context.Context, id shared.ID) (bool, error)
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

// ExternalAccounts is the link between a provider's subject and an account here (SI-10).
//
// `account_identity` rather than `account.external_subject`: one column cannot say *which* provider
// vouched for a subject, and with providers in the plural that is the whole question. The column
// stays where it is, read by nothing new - a rolling update still finds the accounts it already
// knew - and every link written from here lands in the table.
type ExternalAccounts interface {
	// FindBySubject answers the account a provider's subject already names, or an error
	// wrapping shared.ErrNotFound on the first arrival.
	FindBySubject(ctx context.Context, providerID shared.ID, subject string) (identity.Account, error)

	// LinkSubject writes the link. False means the account was not there to link; the unique
	// index is what refuses a subject already spoken for, and it refuses rather than this method,
	// because two sign-ins racing must not both win.
	LinkSubject(ctx context.Context, providerID, accountID shared.ID, subject string, now time.Time) (bool, error)

	// HasIdentity answers whether the account already signs in through any provider. Such an
	// identity is a credential, and an account that holds one is not connected to a second provider
	// on that provider's word (ADR-0071's addendum, E2).
	HasIdentity(ctx context.Context, accountID shared.ID) (bool, error)
}

// SealedProviderSecret is one row's wrapping, as a rotation needs it: which row, and what is
// sealed on it.
type SealedProviderSecret struct {
	ProviderID shared.ID
	Sealed     crypto.Sealed
}

// IdentityProviderSealing is the re-seal's own slice of the provider store (ADR-0045).
//
// Its read is not FindWithSecret's: a rotation works through every row of its level rather than
// asking for one by identifier, and with providers in the plural those are two different questions.
type IdentityProviderSealing interface {
	// ListSealed answers every row of this level that holds a wrapping.
	ListSealed(ctx context.Context) ([]SealedProviderSecret, error)

	// RewrapSecret writes the moved wrapping on one row, guarded by the key it named when it was
	// read. False means the row is gone, or it changed in between.
	RewrapSecret(ctx context.Context, providerID shared.ID, sealed crypto.Sealed, expectedKeyID string) (bool, error)
}
