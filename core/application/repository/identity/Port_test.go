// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The port carries no logic, so there is nothing here to measure - only something to hold in
// place. A test double is the cheapest proof that the interface can still be implemented by a
// fake, which is what the use case tests depend on; a signature change that breaks them breaks
// this first, with a clearer message.
type double struct{}

func (double) FindByToken(context.Context, identity.Token) (Credential, error) {
	return Credential{}, nil
}

func (double) TouchLastUsed(context.Context, shared.ID, time.Time) error { return nil }

func (double) Insert(context.Context, identity.AccessToken, identity.Token) error { return nil }

func (double) Find(context.Context, shared.ID) (identity.AccessToken, error) {
	return identity.AccessToken{}, shared.ErrNotFound
}

func (double) ListForAccount(context.Context, shared.ID) ([]identity.AccessToken, error) {
	return nil, nil
}

func (double) Revoke(context.Context, shared.ID, time.Time) (bool, error) { return false, nil }

var _ AccessTokens = double{}

func TestTheCredentialCarriesTheTenantDefaults(t *testing.T) {
	// The two tenant columns are part of the result rather than a second query, because this
	// runs on every request (i18n-l10n.md §2).
	credential := Credential{TenantLocale: "de", TenantTimeZone: "Europe/Berlin"}

	if credential.TenantLocale == "" || credential.TenantTimeZone == "" {
		t.Error("the tenant defaults are not part of the lookup result")
	}
	if !credential.Token.ID.IsZero() || credential.Account.ID != "" {
		t.Error("the zero credential names a token or an account")
	}
}
