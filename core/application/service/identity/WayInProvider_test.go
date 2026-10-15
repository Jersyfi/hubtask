// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// ADR-0078 §6, UC-ID-04 check 9: a provider connected to an account that already existed is told
// to the person - through every path that connects one.

func (t *toldOf) connected(test *testing.T, f *oidcFixture) {
	test.Helper()
	if len(t.notices) != 1 {
		test.Fatalf("%d notices queued, want one: %+v", len(t.notices), t.notices)
	}
	got := t.notices[0]
	if got.change.Event != domain.WayInProviderConnected || got.account != account || got.tenant != tenant ||
		got.change.ProviderID != f.provider.ID || got.change.ProviderName != f.provider.DisplayName {
		test.Errorf("queued %+v, want the provider's connection to the account", got)
	}
}

func TestAProviderConnectedWithThePasswordIsTold(t *testing.T) {
	f := linkFixture(t, false)
	told := &toldOf{}
	f.writer.WayIn = told
	result, err := arrive(t, f)
	if err != nil || result.Challenge == nil {
		t.Fatalf("arriving: (%+v, %v)", result, err)
	}
	if len(told.notices) != 0 {
		t.Fatalf("told before anything was connected: %+v", told.notices)
	}
	if _, err := (CompleteLink{Writer: f.writer}).Execute(t.Context(), CompleteLinkCommand{
		PendingToken: result.Challenge.Token, Password: secret.New("correct horse battery"),
	}); err != nil {
		t.Fatalf("the link step: %v", err)
	}
	told.connected(t, f)
}

// With a second factor the connection happens at the code, and so does the notice.
func TestAProviderConnectedAtTheSecondFactorIsTold(t *testing.T) {
	f := linkFixture(t, true)
	told := &toldOf{}
	f.writer.WayIn = told
	f.session.writer.Connector = f.writer
	result, err := arrive(t, f)
	if err != nil || result.Challenge == nil {
		t.Fatalf("arriving: (%+v, %v)", result, err)
	}
	next, err := (CompleteLink{Writer: f.writer}).Execute(t.Context(), CompleteLinkCommand{
		PendingToken: result.Challenge.Token, Password: secret.New("correct horse battery"),
	})
	if err != nil || next.Challenge == nil {
		t.Fatalf("the link step: (%+v, %v)", next, err)
	}
	if len(told.notices) != 0 {
		t.Fatalf("told before the second factor connected anything: %+v", told.notices)
	}
	material := f.session.writer.Encryptor.(*encryptorFake).sealedBy[string(mfaSecretPurpose(account))]
	if _, _, err := (CompleteSignIn{Writer: f.session.writer}).Execute(t.Context(), CompleteSignInCommand{
		PendingToken: next.Challenge.Token,
		Code:         domain.TotpCode([]byte(material), domain.TotpStep(now)+1),
	}); err != nil {
		t.Fatalf("the second factor's step: %v", err)
	}
	told.connected(t, f)
}

// SC-33's link by mail connects the provider, and the person is told.
func TestAProviderConnectedByMailIsTold(t *testing.T) {
	f := connectFixture(t)
	told := &toldOf{}
	f.writer.WayIn = told
	f.session.writer.Connector = f.writer
	f.session.withAccount("bert@example.org", "correct horse battery")
	link, _ := connectLinkFor(t, f, account, connectAt.Add(domain.ResetLifetime), 1)

	if _, err := completeConnect(t, f, startConnect(t, f, link)); err != nil {
		t.Fatalf("connecting: %v", err)
	}
	told.connected(t, f)
}

// A provider authoritative for the address connects an existing account that holds no credential,
// and the person is told.
func TestAnExistingAccountConnectedByAnAuthoritativeProviderIsTold(t *testing.T) {
	f := newOidcFixture(t, now, domain.Account{
		ID: account, TenantID: tenant, Kind: domain.AccountUser,
		Email: "ada@example.org", DisplayName: "Ada", Status: domain.AccountActive,
	})
	told := &toldOf{}
	f.writer.WayIn = told
	f.relying.identity.AddressAuthoritative = true

	if _, err := arrive(t, f); err != nil {
		t.Fatalf("arriving: %v", err)
	}
	told.connected(t, f)
}

// A newcomer the provider brings in has had no way in to change: nothing is told (D3).
func TestANewcomerTheProviderBringsInIsNotTold(t *testing.T) {
	f := newOidcFixture(t, now)
	told := &toldOf{}
	f.writer.WayIn = told
	f.store.rows[0].Provisioning = domain.ProvisionAny

	if _, err := arrive(t, f); err != nil {
		t.Fatalf("arriving: %v", err)
	}
	if len(f.accounts.inserted) != 1 {
		t.Fatalf("%d accounts provisioned", len(f.accounts.inserted))
	}
	if len(told.notices) != 0 {
		t.Errorf("a newcomer was told: %+v", told.notices)
	}
}
