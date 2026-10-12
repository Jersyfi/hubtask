// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	stepupport "github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// identity.md §4.1: where a workspace's methods leave the password out, the password stays a way in
// for an account no provider can ever recognise - a sign-in name and no address (ADR-0074, P-16).
// An address is refused before any lookup, as for everybody; an account found by name that also
// holds an address is refused once the password has proved right.

const managedNewPassword = "seven blue lanterns above the quiet harbour"

// managedAlsoAddressed gives Lena an address beside her name - PH-07's upgrade - so a provider can
// reach her.
func (f *stepFixture) managedAlsoAddressed() {
	found := f.session.accounts.byEmail["lena"]
	found.Account.Email = "lena@example.org"
	f.session.accounts.byEmail["lena"] = found
	held := f.passwords.accounts.rows[account]
	held.Account.Email = "lena@example.org"
	f.passwords.accounts.rows[account] = held
}

// passwordLockedOff takes the password out of the methods at the installation, locked.
func passwordLockedOff(f *passwordFixture) {
	f.instance.level.Policy.Patch.Methods = &[]string{domain.MethodOidc}
	f.instance.level.Policy.Locks[domain.SwitchMethods] = true
	f.providers.rows = append(f.providers.rows, domain.IdentityProvider{
		ID: rulesProviderRow, TenantID: tenant, Kind: domain.KindGeneric,
		DisplayName: "id.acme.example", Issuer: "https://id.acme.example", Enabled: true,
	})
}

func TestASignInNameKeepsItsPasswordWhereThePasswordIsOff(t *testing.T) {
	for name, switchOff := range map[string]func(*passwordFixture){
		"the workspace switched it off":  passwordOff,
		"the installation locked it off": passwordLockedOff,
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newStepFixture(now)
			fixture.session.withAccount("bert@example.org", "correct horse battery")
			fixture.withManaged(now.Add(-time.Hour))
			switchOff(fixture.passwords)
			registry := signInRegistry(t, fixture.session.writer)

			// The address meets the workspace's refusal, the same for a known and an unknown one.
			for _, address := range []string{"bert@example.org", "nobody@example.org"} {
				if _, err := signInWith(t, registry, address, "correct horse battery"); detailOf(err) != "auth.password_not_offered" {
					t.Errorf("%s answered %v, want auth.password_not_offered", address, err)
				}
			}
			// A name goes on to the lookup: an unknown one meets the one refusal every failure gets.
			if _, err := signInWith(t, registry, "nobody", startPassword); detailOf(err) != "auth.sign_in_failed" {
				t.Errorf("an unknown name answered %v, want auth.sign_in_failed", err)
			}

			// Lena's start password leads into the change step, under the workspace's rule, and the
			// change is the sign-in.
			out, err := signInWith(t, registry, "Lena", startPassword)
			if err != nil {
				t.Fatalf("signing in with the name: %v", err)
			}
			methods, _ := out["methods"].([]any)
			if len(methods) != 1 || methods[0] != methodPasswordChange {
				t.Fatalf("the start password answered %v, want the change step", out)
			}
			if _, err := (SetPasswordAndSignIn{Writer: fixture.passwords.writer}).Execute(t.Context(),
				SetPasswordAndSignInCommand{
					PendingToken: secret.New(out.String("pending_token")), Password: secret.New("short"),
				}); !errors.Is(err, shared.ErrValidation) {
				t.Errorf("a password the workspace's rule refuses answered %v", err)
			}
			pair, err := SetPasswordAndSignIn{Writer: fixture.passwords.writer}.Execute(t.Context(),
				SetPasswordAndSignInCommand{
					PendingToken: secret.New(out.String("pending_token")), Password: secret.New(managedNewPassword),
				})
			if err != nil || pair.Pair == nil || pair.Pair.Session.ID.IsZero() {
				t.Fatalf("the change step did not sign in: %v", err)
			}
		})
	}
}

// The factor rule holds for a sign-in name as for anybody: where the workspace demands one, the
// start password leads into the enrolment first.
func TestASignInNameMeetsTheFactorRuleWhereThePasswordIsOff(t *testing.T) {
	fixture := newStepFixture(now)
	fixture.withManaged(now.Add(-time.Hour))
	passwordOff(fixture.passwords)
	fixture.passwords.instance.level.Policy.Patch.MfaRequiredFor = requirementOf(domain.MfaForEveryone)

	out, err := signInWith(t, signInRegistry(t, fixture.session.writer), "lena", startPassword)
	if err != nil {
		t.Fatalf("signing in with the name: %v", err)
	}
	methods, _ := out["methods"].([]any)
	if len(methods) != 1 || methods[0] != methodEnroll {
		t.Fatalf("a demanded factor answered %v, want the enrolment", out)
	}
}

// An account found by its name that also holds an address has a provider's way in: refused like an
// address, but only once the password proved right - a wrong one is the one refusal of every failure.
func TestANameWhoseAccountHoldsAnAddressIsRefusedOnceThePasswordProved(t *testing.T) {
	fixture := newStepFixture(now)
	fixture.withManaged(now.Add(-time.Hour))
	fixture.managedAlsoAddressed()
	passwordOff(fixture.passwords)
	registry := signInRegistry(t, fixture.session.writer)

	if _, err := signInWith(t, registry, "lena", "wrong"); detailOf(err) != "auth.sign_in_failed" {
		t.Errorf("a wrong password answered %v, want auth.sign_in_failed", err)
	}
	if _, err := signInWith(t, registry, "lena", startPassword); detailOf(err) != "auth.password_not_offered" {
		t.Errorf("the right password answered %v, want auth.password_not_offered", err)
	}
	if len(fixture.session.sessions.inserted) != 0 || len(fixture.pending.rows) != 0 {
		t.Error("the refused sign-in left a session or a pending step behind")
	}
}

// The change step a sign-in owed: open for an account only the password reaches, refused for one
// that also holds an address.
func TestTheChangeStepStaysOpenOnlyForAnAccountThePasswordAloneReaches(t *testing.T) {
	for name, c := range map[string]struct {
		addressed bool
		want      string
	}{
		"a sign-in name alone":          {false, ""},
		"a sign-in name and an address": {true, "auth.password_not_offered"},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newStepFixture(now)
			fixture.withManaged(now.Add(-time.Hour))
			out, err := signInWith(t, signInRegistry(t, fixture.session.writer), "lena", startPassword)
			if err != nil {
				t.Fatalf("signing in: %v", err)
			}
			passwordOff(fixture.passwords)
			if c.addressed {
				fixture.managedAlsoAddressed()
			}

			_, err = SetPasswordAndSignIn{Writer: fixture.passwords.writer}.Execute(t.Context(),
				SetPasswordAndSignInCommand{
					PendingToken: secret.New(out.String("pending_token")), Password: secret.New(managedNewPassword),
				})
			if detailOf(err) != c.want {
				t.Errorf("the change step answered %v, want %q", err, c.want)
			}
		})
	}
}

// The step-up offers and takes the password of an account only the password reaches, where the
// workspace switched it off - and still not that of an account with an address.
func TestTheStepUpTakesTheManagedPersonsPasswordWhereThePasswordIsOff(t *testing.T) {
	for name, c := range map[string]struct {
		addressed bool
		offered   bool
	}{
		"a sign-in name alone":          {false, true},
		"a sign-in name and an address": {true, false},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newStepFixture(now)
			fixture.session.writer.StepUps = newStepUps()
			credential, _ := refreshCredential(now)
			fixture.session.sessions.sessions[sessionRowID] = repository.SessionCredential{
				TenantStatus: domain.TenantActive, Session: credential.Session, Account: credential.Account,
			}
			fixture.withManaged(now.Add(-time.Hour))
			if c.addressed {
				fixture.managedAlsoAddressed()
			}
			fixture.session.writer.People = newAccounts(fixture.passwords.accounts.rows[account].Account)
			passwordOff(fixture.passwords)

			methods, err := StepUpVerifier{Writer: fixture.session.writer}.Methods(t.Context(), tenant, account)
			if err != nil {
				t.Fatalf("reading the methods: %v", err)
			}
			offered := false
			for _, method := range methods {
				offered = offered || method == stepupport.MethodPassword
			}
			if offered != c.offered {
				t.Errorf("the password offered: %v, want %v", offered, c.offered)
			}

			grant, err := StepUp{Writer: fixture.session.writer}.Execute(t.Context(), signedInActor(),
				StepUpCommand{Password: secret.New(startPassword)})
			if c.offered && (err != nil || grant.Method != domain.StepUpPassword) {
				t.Errorf("the managed person's step-up answered %+v, %v", grant, err)
			}
			if !c.offered && detailOf(err) != "auth.password_not_offered" {
				t.Errorf("the addressed account's step-up answered %v, want auth.password_not_offered", err)
			}
		})
	}
}

// The deprecated factor-off proof is a password door too: open for the managed person.
func TestTheManagedPersonTurnsTheFactorOffWithThePasswordWhereThePasswordIsOff(t *testing.T) {
	fixture := newStepFixture(now)
	fixture.withManaged(now.Add(-time.Hour))
	fixture.session.writer.Encryptor = newEncryptor()
	fixture.session.writer.People = newAccounts(fixture.passwords.accounts.rows[account].Account)
	enrolled(t, fixture.session)
	passwordOff(fixture.passwords)

	if err := (DisableTotp{Writer: fixture.session.writer}).Execute(t.Context(), signedInActor(),
		DisableTotpCommand{Password: secret.New(startPassword)}); err != nil {
		t.Fatalf("turning the factor off with the password answered %v", err)
	}
	if _, err := fixture.enroll.Find(t.Context(), account); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the factor is still armed: %v", err)
	}
}
