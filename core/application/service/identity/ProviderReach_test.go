// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// SC-33 (#1140, ADR-0078 §1, UC-ID-12): the password switch says, before the password goes off, how
// many people here no provider switched on here signs in - connected, invited and service accounts
// aside. A number, through the same permission as the list of providers.

// reachFixture is a workspace with its own provider switched on, an ended installation offer, and
// five accounts: one connected to the provider, one connected only to the ended offer, one with
// nothing, one invited and one service account.
func reachFixture(t *testing.T) (*providerFixture, CountAccountsWithoutProvider) {
	t.Helper()
	at := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	f := newProviderFixture(at)
	f.store.rows = []domain.IdentityProvider{ownProvider, offeredUntil(at.Add(-time.Hour))}

	member := func(id, email string, kind domain.AccountKind, status domain.AccountStatus) domain.Account {
		return domain.Account{
			ID: shared.ID(id), TenantID: tenant, Kind: kind, Email: email, DisplayName: email, Status: status,
		}
	}
	connected := member("01936f2a-7c1e-7000-8000-0000000033c1", "connected@example.org", domain.AccountUser, domain.AccountActive)
	ended := member("01936f2a-7c1e-7000-8000-0000000033c2", "ended@example.org", domain.AccountUser, domain.AccountActive)
	none := member("01936f2a-7c1e-7000-8000-0000000033c3", "none@example.org", domain.AccountUser, domain.AccountActive)
	invited := member("01936f2a-7c1e-7000-8000-0000000033c4", "invited@example.org", domain.AccountUser, domain.AccountInvited)
	robot := member("01936f2a-7c1e-7000-8000-0000000033c5", "", domain.AccountServiceAccount, domain.AccountActive)

	external := newExternal(newAccounts(connected, ended, none, invited, robot))
	external.bySubject[linkKey(ownProvider.ID, "connected")] = connected
	external.bySubject[linkKey(fallbackRow, "ended")] = ended
	return f, CountAccountsWithoutProvider{Writer: f.writer, External: external}
}

func TestTheSwitchCountsThePeopleNoProviderHereSignsIn(t *testing.T) {
	f, handler := reachFixture(t)

	counted, err := handler.Execute(t.Context(), providerActor())
	if err != nil {
		t.Fatalf("counting: %v", err)
	}
	// The one with nothing and the one whose only identity is at the ended offer; never the
	// connected one, the invitation or the service account.
	if counted != 2 {
		t.Errorf("counted %d, want 2", counted)
	}
	if len(f.auth.requests) != 1 || f.auth.requests[0].Permission != service.PermissionManageMembers ||
		f.auth.requests[0].Alternative != service.PermissionReadConfiguration {
		t.Errorf("the authoriser was asked %+v, want the providers' list's permission", f.auth.requests)
	}

	// With the workspace's provider switched off too, every active person counts.
	f.store.rows[0].Enabled = false
	if counted, _ := handler.Execute(t.Context(), providerActor()); counted != 3 {
		t.Errorf("with no provider on counted %d, want every active person", counted)
	}
}

func TestARefusedCallerCountsNothing(t *testing.T) {
	f, handler := reachFixture(t)
	f.auth.refuse = shared.ErrForbidden.WithDetail("access.not_permitted")

	if _, err := handler.Execute(t.Context(), providerActor()); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a refused caller answered %v", err)
	}
}

// Through the registry, which is what REST, MCP and automation all meet first.
func TestTheCountGoesThroughTheRegistry(t *testing.T) {
	_, handler := reachFixture(t)
	registry, err := usecase.NewRegistry(nil, handler.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	out, err := registry.Invoke(t.Context(), CountAccountsWithoutProviderName, providerActor(), usecase.Input{})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}
	if out["count"] != 2 {
		t.Errorf("the registry answered %v, want the count", out)
	}
}
