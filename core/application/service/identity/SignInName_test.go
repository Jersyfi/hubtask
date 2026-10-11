// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"fmt"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// UC-ID-20 checks 3 and 4: a managed account signs in with its sign-in name, and every failure -
// a name or an address, unknown or with the wrong password, or a name no account could hold - is
// the one refusal, counted on the same kind of ledger subject (T-02).

const startPassword = "test-start-password"

// withManaged puts Lena behind her sign-in name, holding a start password.
func (f *stepFixture) withManaged(drawnAt time.Time) {
	f.session.accounts.byEmail["lena"] = repository.SignInAccount{
		TenantStatus: domain.TenantActive,
		Account: domain.Account{
			ID: account, TenantID: tenant, Kind: domain.AccountUser,
			SignInName: "lena", DisplayName: "Lena", Status: domain.AccountActive,
		},
		PasswordHash: secret.New("hash:" + startPassword), TenantLocale: "en", TenantTimeZone: "UTC",
	}
	held := repository.PasswordAccount{
		Account: domain.Account{
			ID: account, TenantID: tenant, Kind: domain.AccountUser,
			SignInName: "lena", DisplayName: "Lena", Status: domain.AccountActive,
		},
		PasswordHash: secret.New("hash:" + startPassword), PasswordSetAt: drawnAt, StartPasswordAt: drawnAt,
	}
	f.passwords.accounts.rows[account] = held
}

func signInRegistry(t *testing.T, writer SessionWriter) *usecase.Registry {
	t.Helper()
	registry, err := usecase.NewRegistry(nil, SignIn{Writer: writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	return registry
}

func signInWith(t *testing.T, registry *usecase.Registry, identifier, password string) (usecase.Output, error) {
	t.Helper()
	return signInFrom(t, registry, identifier, password, "203.0.113.7:9")
}

func signInFrom(
	t *testing.T, registry *usecase.Registry, identifier, password, remote string,
) (usecase.Output, error) {
	t.Helper()
	return registry.Invoke(t.Context(), SignInName, appshared.Anonymous("en", "UTC"), usecase.Input{
		"identifier": identifier, "password": password, "remote_addr": remote,
	})
}

// The refusal as a client meets it: category, code, detail, parameters and field errors.
func refusalShape(err error) string {
	refusal := shared.AsError(err)
	if refusal == nil {
		return fmt.Sprintf("not a refusal: %v", err)
	}
	return fmt.Sprintf("%s|%s|%s|%v|%v", refusal.Category, refusal.Code, refusal.DetailCode,
		refusal.Params, refusal.Fields)
}

func TestASignInNameAndAnAddressAreRefusedAlike(t *testing.T) {
	fixture := newStepFixture(now)
	fixture.session.withAccount("bert@example.org", "correct horse battery")
	fixture.withManaged(now.Add(-time.Hour))
	registry := signInRegistry(t, fixture.session.writer)

	cases := []struct{ name, identifier, password, subject string }{
		{"an unknown name", "nobody", startPassword, "account:nobody"},
		{"an unknown address", "nobody@example.org", "correct horse battery", "account:nobody@example.org"},
		{"a known name, a wrong password", "Lena", "wrong", "account:lena"},
		{"a known address, a wrong password", "Bert@Example.org", "wrong", "account:bert@example.org"},
		{"a name too short to exist", "ab", "anything", "account:ab"},
		{"a name with a space", "le na", "anything", "account:le na"},
	}
	var first string
	for index, c := range cases {
		// Each from its own network, so the IP class's own ledger stays out of the comparison.
		_, err := signInFrom(t, registry, c.identifier, c.password, fmt.Sprintf("10.0.%d.7:9", index))
		shape := refusalShape(err)
		if first == "" {
			first = shape
			if !errors.Is(err, shared.ErrUnauthenticated) || shared.AsError(err).DetailCode != "auth.sign_in_failed" {
				t.Fatalf("%s answered %s, want the one refusal", c.name, shape)
			}
		} else if shape != first {
			t.Errorf("%s answered %s, %s answered %s", c.name, shape, cases[0].name, first)
		}
		if failures := fixture.session.attempts.standing[c.subject].Failures; failures != 1 {
			t.Errorf("%s: the ledger counts %d failures on %q, want 1", c.name, failures, c.subject)
		}
	}
}

// A locked name meets the lockout an address meets, with the same answer.
func TestALockedNameIsLockedLikeAnAddress(t *testing.T) {
	fixture := newStepFixture(now)
	fixture.session.withAccount("bert@example.org", "correct horse battery")
	fixture.withManaged(now.Add(-time.Hour))
	registry := signInRegistry(t, fixture.session.writer)
	for _, subject := range []string{"account:lena", "account:bert@example.org"} {
		fixture.session.attempts.standing[subject] = repository.AuthAttempt{
			Failures: 9, LockedUntil: now.Add(time.Minute),
		}
	}

	_, byName := signInWith(t, registry, "lena", startPassword)
	_, byAddress := signInWith(t, registry, "bert@example.org", "correct horse battery")
	if shared.AsError(byName).DetailCode != "auth.sign_in_locked" || refusalShape(byName) != refusalShape(byAddress) {
		t.Errorf("the name answered %s, the address %s", refusalShape(byName), refusalShape(byAddress))
	}
}

// Check 3: the start password, typed with the name in any spelling, leads into choosing the
// person's own password - and choosing it is the sign-in.
func TestAManagedAccountSignsInWithItsNameAndChoosesItsPassword(t *testing.T) {
	fixture := newStepFixture(now)
	fixture.session.writer.Text = text.Composing{}
	fixture.withManaged(now.Add(-time.Hour))
	registry := signInRegistry(t, fixture.session.writer)

	out, err := signInWith(t, registry, "  LENA ", startPassword)
	if err != nil {
		t.Fatalf("signing in with the name: %v", err)
	}
	methods, _ := out["methods"].([]any)
	if out["mfa_required"] != true || len(methods) != 1 || methods[0] != methodPasswordChange {
		t.Fatalf("the start password answered %v, want the change step", out)
	}

	pair, err := SetPasswordAndSignIn{Writer: fixture.passwords.writer}.Execute(t.Context(),
		SetPasswordAndSignInCommand{
			PendingToken: secret.New(out.String("pending_token")),
			Password:     secret.New("seven blue lanterns above the quiet harbour"),
		})
	if err != nil || pair.Session.ID.IsZero() {
		t.Fatalf("choosing the password did not sign in: %v", err)
	}
}

// The sign-in name is a word the person types beside the password, so the new one may not carry it.
func TestTheNewPasswordMayNotCarryTheName(t *testing.T) {
	words := domain.ContextWords(nil, domain.PasswordContext{SignInName: "lena-mueller"})
	found := false
	for _, word := range words {
		if word == "lenamueller" || word == "lena-mueller" {
			found = true
		}
	}
	if !found {
		t.Errorf("the context words %v leave the sign-in name out", words)
	}
}

// Exactly one identifier: neither, or both, is a refusal of the request's shape, before anything
// is looked up.
func TestASignInCarriesExactlyOneIdentifier(t *testing.T) {
	fixture := newStepFixture(now)
	registry := signInRegistry(t, fixture.session.writer)
	for name, in := range map[string]usecase.Input{
		"neither": {"password": "x"},
		"both":    {"password": "x", "identifier": "lena", "email": "bert@example.org"},
	} {
		_, err := registry.Invoke(t.Context(), SignInName, appshared.Anonymous("en", "UTC"), in)
		if shared.AsError(err).DetailCode != "auth.identifier_one" {
			t.Errorf("%s answered %v", name, err)
		}
	}
	if len(fixture.session.attempts.standing) != 0 {
		t.Error("a refusal of the shape advanced the ledger")
	}
}
