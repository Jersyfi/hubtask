// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// ADR-0075 §2, UC-ID-05 check 5, UC-ID-03 check 6: an account that signs in only through a provider
// proves a step-up with a fresh sign-in there. The proof counts only for the identity already
// connected to this account, only with an `auth_time` inside the step-up's window, and only on the
// session that asked.

// providerStepUpFixture is a provider-only person: no password, no factor, connected to the
// workspace's one provider, signed in on a live session.
func providerStepUpFixture(t *testing.T) *oidcFixture {
	t.Helper()
	f := newOidcFixture(t, now, domain.Account{
		ID: account, TenantID: tenant, Kind: domain.AccountUser,
		Email: "bert@example.org", DisplayName: "Bert", Status: domain.AccountActive,
	})
	if _, err := f.external.LinkSubject(t.Context(), oidcProviderRow, account, "bert-at-the-provider", now); err != nil {
		t.Fatalf("connecting the account: %v", err)
	}
	f.external.links = nil
	f.relying.identity.Subject = "bert-at-the-provider"
	f.relying.identity.AuthTime = now.Add(-10 * time.Second)

	session := f.session
	session.writer.StepUps = newStepUps()
	credential, _ := refreshCredential(now)
	session.sessions.sessions[sessionRowID] = repository.SessionCredential{
		TenantStatus: domain.TenantActive, Session: credential.Session, Account: credential.Account,
	}
	session.writer.StepUpProviders = ProviderStepUps{
		Providers: f.store, Flows: f.flows, External: f.external,
		Relying: f.relying, RedirectURL: f.writer.RedirectURL,
	}
	return f
}

// stepUpAtTheProvider runs both halves and answers the second's result.
func stepUpAtTheProvider(t *testing.T, f *oidcFixture) (StepUpGrant, error) {
	t.Helper()
	started, err := StartProviderStepUp{Writer: f.session.writer}.Execute(t.Context(), signedInActor())
	if err != nil {
		t.Fatalf("starting the step-up at the provider: %v", err)
	}
	if started.ProviderName != f.provider.DisplayName || started.URL == "" {
		t.Errorf("the start answered %+v", started)
	}
	return StepUp{Writer: f.session.writer}.Execute(t.Context(), signedInActor(), StepUpCommand{
		State: secret.New(f.relying.asked.State), AuthorizationCode: "the-code",
	})
}

func TestAFreshSignInAtTheConnectedProviderProvesAStepUp(t *testing.T) {
	f := providerStepUpFixture(t)

	grant, err := stepUpAtTheProvider(t, f)
	if err != nil {
		t.Fatalf("stepping up at the provider: %v", err)
	}
	if grant.Method != domain.StepUpProvider {
		t.Errorf("method %q, want PROVIDER", grant.Method)
	}
	// The provider was asked to sign the person in again, not to answer from its own session.
	if !f.relying.asked.Fresh {
		t.Error("the authorization request did not ask for a fresh sign-in")
	}
	// And it is the same proof every method answers: one privileged action consumes it.
	verifier := StepUpVerifier{Writer: f.session.writer}
	if ok, err := verifier.Satisfied(t.Context(), account, grant.Token.Reveal()); err != nil || !ok {
		t.Fatalf("the proof did not satisfy (%v, %v)", ok, err)
	}
}

// A stolen session alone proves nothing: the demand refuses and names the provider - which is what
// the dialog builds "Confirm with <name>" from - and a state minted for another session finishes
// nothing here.
func TestTheSessionAloneProvesNothingAtTheProvider(t *testing.T) {
	f := providerStepUpFixture(t)
	verifier := StepUpVerifier{Writer: f.session.writer}

	err := stepup.Demand(t.Context(), verifier, tenant, account, "")
	if methods := demandedMethods(t, err); methods != "PROVIDER" {
		t.Errorf("a provider-only account is asked for %q, want PROVIDER", methods)
	}
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.Params["provider"] != f.provider.DisplayName {
		t.Errorf("the refusal names the provider %q, want %q", refusal.Params["provider"], f.provider.DisplayName)
	}

	// The step-up is started on this session and finished on another: the other finds no flow.
	if _, err := (StartProviderStepUp{Writer: f.session.writer}).Execute(t.Context(), signedInActor()); err != nil {
		t.Fatalf("starting: %v", err)
	}
	credential, _ := refreshCredential(now)
	other := credential.Session
	other.ID = refreshRowID
	f.session.sessions.sessions[refreshRowID] = repository.SessionCredential{
		TenantStatus: domain.TenantActive, Session: other, Account: credential.Account,
	}
	thief := signedInActor()
	thief.TokenID = refreshRowID
	_, err = StepUp{Writer: f.session.writer}.Execute(t.Context(), thief, StepUpCommand{
		State: secret.New(f.relying.asked.State), AuthorizationCode: "the-code",
	})
	if err == nil || !strings.Contains(err.Error(), "auth.oidc_failed") {
		t.Fatalf("another session finished this session's step-up: %v", err)
	}
}

// A different identity at the same provider is refused - and never connected.
func TestADifferentIdentityAtTheProviderIsRefused(t *testing.T) {
	f := providerStepUpFixture(t)
	f.relying.identity.Subject = "somebody-else-at-the-provider"

	_, err := stepUpAtTheProvider(t, f)
	if err == nil || !strings.Contains(err.Error(), "auth.step_up_provider_mismatch") {
		t.Fatalf("another identity at the provider answered %v", err)
	}
	if len(f.external.links) != 0 {
		t.Errorf("the refused identity was connected: %v", f.external.links)
	}
	if got := f.session.attempts.standing[stepUpSubject(account)].Failures; got != 1 {
		t.Errorf("the ledger stands at %d, want 1", got)
	}
}

// A provider that omits auth_time, or answers one older than the step-up's window, proves nothing
// fresh, and the refusal says so in a sentence rather than trusting the round trip.
func TestAProviderWithoutAFreshAuthTimeIsRefused(t *testing.T) {
	for name, authTime := range map[string]time.Time{
		"no auth_time":    {},
		"a stale one":     now.Add(-time.Hour),
		"one from beyond": now.Add(time.Hour),
	} {
		t.Run(name, func(t *testing.T) {
			f := providerStepUpFixture(t)
			f.relying.identity.AuthTime = authTime

			_, err := stepUpAtTheProvider(t, f)
			if err == nil || !strings.Contains(err.Error(), "auth.step_up_provider_not_fresh") {
				t.Fatalf("%s answered %v", name, err)
			}
			// A refused proof counts, as a wrong code does (SC-22).
			if got := f.session.attempts.standing[stepUpSubject(account)].Failures; got != 1 {
				t.Errorf("%s left the ledger at %d, want 1", name, got)
			}
		})
	}
}

// A provider switched off in the workspace is not a way to prove anything here.
func TestAProviderSwitchedOffIsNotOffered(t *testing.T) {
	f := providerStepUpFixture(t)
	f.store.rows[0].Enabled = false

	err := stepup.Demand(t.Context(), StepUpVerifier{Writer: f.session.writer}, tenant, account, "")
	if methods := demandedMethods(t, err); methods != "" {
		t.Errorf("with the provider off the account is asked for %q, want nothing", methods)
	}
	_, err = StartProviderStepUp{Writer: f.session.writer}.Execute(t.Context(), signedInActor())
	if err == nil || !strings.Contains(err.Error(), "auth.step_up_no_provider") {
		t.Fatalf("starting with the provider off answered %v", err)
	}
}

// Switched off while the person was at the provider: the return proves nothing. An installation's
// provider, whose row stays on and whose switch is the workspace's - the one a re-read of the row
// alone would miss.
func TestAProviderSwitchedOffDuringTheRoundTripProvesNothing(t *testing.T) {
	f := providerStepUpFixture(t)
	offered := shared.MustParseID("22222222-2222-4222-8222-222222222222")
	configureFixtureProvider(t, f, offered, shared.ID(""), "https://login.platform.example", now)
	f.store.rows = f.store.rows[1:] // only the installation's: the workspace's own is not the way in here
	if _, err := f.external.LinkSubject(t.Context(), offered, account, "bert-at-the-provider", now); err != nil {
		t.Fatalf("connecting: %v", err)
	}
	workspaces := &workspaceStore{row: domain.Workspace{Tenant: domain.Tenant{ID: tenant}, Version: 1}}
	workspaces.row.Settings = workspaces.row.Settings.WithOffer(offered, true)
	f.session.writer.StepUpProviders.Workspaces = workspaces

	if _, err := (StartProviderStepUp{Writer: f.session.writer}).Execute(t.Context(), signedInActor()); err != nil {
		t.Fatalf("starting: %v", err)
	}
	workspaces.row.Settings = workspaces.row.Settings.WithOffer(offered, false)

	_, err := StepUp{Writer: f.session.writer}.Execute(t.Context(), signedInActor(), StepUpCommand{
		State: secret.New(f.relying.asked.State), AuthorizationCode: "the-code",
	})
	if err == nil || !strings.Contains(err.Error(), "auth.step_up_no_provider") {
		t.Fatalf("a provider switched off during the round trip answered %v", err)
	}
}
