// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-INS-08 check 2: a suspended workspace refuses work with the sentence that says it is paused -
// at the provider's door too, after the provider's word and before any session is opened, as the
// password, the second step and the passkey refuse it. Resumed, the same person signs in.
func TestAnOidcSignInIntoASuspendedWorkspaceIsRefused(t *testing.T) {
	at := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	ada := domain.Account{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000006a2"), TenantID: tenant,
		Kind: domain.AccountUser, Email: "ada@example.org", DisplayName: "Ada", Status: domain.AccountActive,
	}
	f := newOidcFixture(t, at, ada)
	f.store.rows = nil
	ownRow := shared.ID("01936f2a-7c1e-7000-8000-0000000006a1")
	configureFixtureProvider(t, f, ownRow, tenant, "https://id.acme.example", at)
	f.external.bySubject[linkKey(ownRow, "provider-subject-1")] = ada
	workspaces := &workspaceStore{row: domain.Workspace{Tenant: domain.Tenant{ID: tenant, Slug: "acme", Status: domain.TenantSuspended}}}
	f.writer.Workspaces = workspaces

	arrive := func(code string) (SignInResult, error) {
		authorization, err := StartOidcSignIn{Writer: f.writer}.Execute(t.Context(), StartOidcSignInCommand{ProviderID: ownRow})
		if err != nil {
			t.Fatalf("starting: %v", err)
		}
		return CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(), CompleteOidcSignInCommand{Code: code, State: authorization.State})
	}

	sessions := len(f.session.sessions.inserted)
	if _, err := arrive("one"); detailOf(err) != "access.tenant_suspended" {
		t.Fatalf("a suspended workspace's provider sign-in answered %v", err)
	}
	if len(f.session.sessions.inserted) != sessions {
		t.Fatal("a session was opened into a suspended workspace")
	}

	workspaces.row.Status = domain.TenantActive
	result, err := arrive("two")
	if err != nil || pairOf(result).Session.AccountID != ada.ID {
		t.Fatalf("resumed, the sign-in answered %+v, %v", pairOf(result), err)
	}
}
