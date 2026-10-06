// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"slices"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// ADR-0078 §3 (SC-34): for a provider that is switched on but broken, an operator opens the password
// for one workspace for a limited time. It is the fallback with the cause OPERATOR: the workspace's
// own switch and any installation lock give way to it, it ends on its own, and under it an account
// without a password is mailed a link to set one.

// operatorOpened opens the password in the fixture's workspace until the moment given.
func operatorOpened(f *stepFixture, until time.Time) {
	f.passwords.workspace.row.PasswordOpening = domain.PasswordOpening{
		Until: until, Requester: "TICKET-4711", Reason: "the directory answers 500",
	}
}

// brokenProviderOnly is a workspace that switched the password off and signs in through its own
// provider, which is switched on - and, for the purpose of the test, broken: nothing here can tell.
func brokenProviderOnly(f *wayInFixture) {
	f.passwords.workspace.row.Settings.SignIn.Methods = providersAlone()
	f.passwords.providers.rows = []domain.IdentityProvider{workspaceProvider(rulesProviderRow)}
}

// door asks the password writer once.
func (f *wayInFixture) door(t *testing.T) (bool, FallbackCause) {
	t.Helper()
	open, cause, err := f.passwords.writer.PasswordOpen(t.Context(), tenant)
	if err != nil {
		t.Fatalf("asking the door: %v", err)
	}
	return open, cause
}

// recordedCause answers the cause of the account's fallback entry, empty where there is none.
func (f *wayInFixture) recordedCause() any {
	for _, entry := range f.session.audit.entries {
		if entry.Action == PasswordFallbackAction && entry.TenantID == tenant && entry.ActorID == account {
			return fallbackCause(entry)
		}
	}
	return nil
}

func TestAnOperatorsOpeningOverridesTheWorkspacesSwitch(t *testing.T) {
	f := newWayInFixture()
	brokenProviderOnly(f)

	// A provider is on, so nothing falls back on its own: the password is shut.
	if open, cause := f.door(t); open || cause.Opens() {
		t.Fatalf("with the provider on the door answered open %v, cause %q", open, cause)
	}

	operatorOpened(f.stepFixture, now.Add(24*time.Hour))
	if open, cause := f.door(t); !open || cause != FallbackCauseOperator {
		t.Fatalf("under the operator's opening the door answered open %v, cause %q", open, cause)
	}
	// The card offers the password first, and keeps the provider's button beside it: the provider is
	// on, and the operator cannot tell from here whether it has come back.
	rules, err := f.card.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
	if err != nil {
		t.Fatalf("reading the card: %v", err)
	}
	if !rules.PasswordFallback ||
		!slices.Equal(rules.Methods, []string{domain.MethodDirect, domain.MethodOidc}) {
		t.Errorf("the card offers %v (fallback %v)", rules.Methods, rules.PasswordFallback)
	}

	if result := f.signsIn(t, "correct horse battery"); result.Pair == nil && result.Challenge == nil {
		t.Fatal("an account with a password did not sign in under the opening")
	}
	if cause := f.recordedCause(); cause != string(FallbackCauseOperator) {
		t.Errorf("the sign-in under the opening is in the trail with cause %v, want OPERATOR", cause)
	}
}

func TestAnOperatorsOpeningOverridesAnInstallationLock(t *testing.T) {
	f := newWayInFixture()
	f.passwords.workspace.row.Settings.SignIn.Methods = &[]string{domain.MethodDirect, domain.MethodOidc}
	f.passwords.instance.level.Policy.Patch.Methods = providersAlone()
	f.passwords.instance.level.Policy.Locks[domain.SwitchMethods] = true
	f.passwords.providers.rows = []domain.IdentityProvider{workspaceProvider(rulesProviderRow)}

	if open, _ := f.door(t); open {
		t.Fatal("the installation's lock without the password left it open")
	}
	operatorOpened(f.stepFixture, now.Add(time.Hour))
	if open, cause := f.door(t); !open || cause != FallbackCauseOperator {
		t.Errorf("under the lock the opening answered open %v, cause %q", open, cause)
	}
}

func TestAnOperatorsOpeningEndsOnItsOwn(t *testing.T) {
	cases := map[string]time.Time{
		"at its end":   now,
		"an hour past": now.Add(-time.Hour),
	}
	for name, until := range cases {
		t.Run(name, func(t *testing.T) {
			f := newWayInFixture()
			brokenProviderOnly(f)
			operatorOpened(f.stepFixture, until)

			if open, cause := f.door(t); open || cause.Opens() {
				t.Errorf("an ended opening left the door open %v, cause %q", open, cause)
			}
			if f.fallsBack(t) {
				t.Error("an ended opening is still on the card")
			}
			_, err := SignIn{Writer: f.session.writer}.Execute(t.Context(), SignInCommand{
				Email: "bert@example.org", Password: secret.New("correct horse battery"),
			})
			if detailOf(err) != "auth.password_not_offered" {
				t.Errorf("after the opening's end the password answered %v", err)
			}
		})
	}
}

// Where the workspace keeps the password on, an opening adds nothing: it is no fallback, and a
// sign-in is recorded as the ordinary one it is.
func TestAnOpeningWhereThePasswordIsOnIsNoFallback(t *testing.T) {
	f := newWayInFixture()
	operatorOpened(f.stepFixture, now.Add(time.Hour))

	if open, cause := f.door(t); !open || cause.Opens() {
		t.Errorf("with the password on the door answered open %v, cause %q", open, cause)
	}
	f.signsIn(t, "correct horse battery")
	if cause := f.recordedCause(); cause != nil {
		t.Errorf("a sign-in where the password is on recorded a fallback with cause %v", cause)
	}
}

// Under the opening, SC-25's first-password link applies to an account connected only to a provider:
// the provider is on, so without the opening the mail points to it - and under the opening, which
// exists because that provider is broken, it carries a link to set a password.
func TestUnderAnOpeningAProviderOnlyAccountIsMailedAFirstPassword(t *testing.T) {
	f := fallbackFixture(t)
	withoutPassword(f)
	store := rulesProviders(ownProvider)
	f.passwords.writer.WaysIn.Providers = store
	f.passwords.writer.Session.StepUpProviders = ProviderStepUps{
		Providers: store, External: connectedTo{providers: []shared.ID{ownProvider.ID}},
		Workspaces: f.passwords.workspace,
	}
	before, err := MintResetToken{Writer: f.passwords.writer}.MintResetToken(t.Context(), tenant, account)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if before.First || !before.Token.IsEmpty() {
		t.Fatalf("with its provider on, a provider-only account was mailed %+v", before)
	}

	operatorOpened(f, now.Add(time.Hour))
	link, err := MintResetToken{Writer: f.passwords.writer}.MintResetToken(t.Context(), tenant, account)
	if err != nil {
		t.Fatalf("minting under the opening: %v", err)
	}
	if !link.First || link.Token.IsEmpty() {
		t.Fatalf("under the opening a provider-only account was mailed %+v, want a first password", link)
	}
	result, err := ResetPassword{Writer: f.passwords.writer}.Execute(t.Context(), ResetPasswordCommand{
		Token: link.Token, Password: secret.New("seven blue lanterns over the harbour"),
	})
	if err != nil || result.Pair == nil {
		t.Fatalf("setting the first password under the opening answered %+v (%v)", result, err)
	}
	var cause any
	for _, entry := range append(f.passwords.audit.entries, f.session.audit.entries...) {
		if entry.Action == PasswordFallbackAction {
			cause = fallbackCause(entry)
		}
	}
	if cause != string(FallbackCauseOperator) {
		t.Errorf("the session the opening let in is in the trail with cause %v, want OPERATOR", cause)
	}
}
