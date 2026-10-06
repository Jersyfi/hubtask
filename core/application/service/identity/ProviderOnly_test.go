// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// UC-ID-05 check 5 (SC-09): an account that signs in only through a provider holds no password, so
// it is neither offered a password change nor asked for a password as a proof. Before SC-09 the
// step-up named the password for everybody, and `/accounts/me` did not say whether there was one.

// providerOnly is the step-up fixture's account with its password taken away.
func providerOnly(t *testing.T) *sessionFixture {
	t.Helper()
	fixture := stepUpFixture(now)
	held := fixture.accounts.byEmail["bert@example.org"]
	held.PasswordHash = secret.Secret{}
	fixture.accounts.byEmail["bert@example.org"] = held
	return fixture
}

func TestAProviderOnlyAccountIsNotAskedForAPassword(t *testing.T) {
	fixture := providerOnly(t)
	verifier := StepUpVerifier{Writer: fixture.writer}
	actor := signedInActor()

	err := stepup.Demand(t.Context(), verifier, actor.TenantID, actor.AccountID, "")
	if methods := demandedMethods(t, err); methods != "" {
		t.Errorf("an account without a password or a factor is asked for %q, want nothing", methods)
	}

	// With a factor armed, the code is the proof, and a recovery code beside it (ADR-0075 §1) -
	// never the password the account does not hold.
	enrolled(t, fixture)
	err = stepup.Demand(t.Context(), verifier, actor.TenantID, actor.AccountID, "")
	if methods := demandedMethods(t, err); methods != "TOTP RECOVERY" {
		t.Errorf("a provider-only account with a factor is asked for %q, want TOTP RECOVERY", methods)
	}
}

func TestTheOwnAccountSaysWhetherItHasAPassword(t *testing.T) {
	for _, c := range []struct {
		name    string
		fixture *sessionFixture
		want    bool
	}{
		{"with a password", stepUpFixture(now), true},
		{"provider only", providerOnly(t), false},
	} {
		handler := GetOwnAccount{
			Accounts: c.fixture.writer.People, UnitOfWork: &unitOfWork{},
			Password: c.fixture.writer,
		}
		actor := signedInActor()
		actor.Scopes = []string{accountsRead}
		own, err := handler.ExecuteWithRecovery(t.Context(), actor)
		if err != nil {
			t.Fatalf("%s: reading: %v", c.name, err)
		}
		if own.HasPassword != c.want {
			t.Errorf("%s: has_password reads %v, want %v", c.name, own.HasPassword, c.want)
		}
	}
}

// UC-ID-06 check 2: a session a provider opened remembers which provider, so the list can name it.
func TestAProviderSessionRemembersItsProvider(t *testing.T) {
	f := newOidcFixture(t, now)
	result, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(), CompleteOidcSignInCommand{
		Code: "the-code", State: start(t, f),
	})
	if err != nil || result.Pair == nil {
		t.Fatalf("the arrival answered (%+v, %v)", result, err)
	}
	if via := result.Pair.Session.SignedInVia; via != f.provider.ID {
		t.Errorf("the session names provider %q, want %q", via, f.provider.ID)
	}
}
