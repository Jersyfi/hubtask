// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	identityrepository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// heldRoles is the membership store as the real authoriser reads it: what one account holds.
type heldRoles struct{ held []identity.Membership }

func (m *heldRoles) Along(context.Context, shared.ID, []identity.Scope) ([]identity.Membership, error) {
	return m.held, nil
}

func (m *heldRoles) SharedItemsIn(context.Context, shared.ID, shared.ID) ([]shared.ID, error) {
	return nil, nil
}

func (m *heldRoles) Administrators(context.Context, []identity.Scope) ([]shared.ID, error) {
	return nil, nil
}

func (m *heldRoles) HubsOf(context.Context, []shared.ID) (map[shared.ID]identityrepository.Hub, error) {
	return nil, nil
}

func (m *heldRoles) HoldsAny(context.Context, shared.ID) (bool, error) { return len(m.held) > 0, nil }

// grantsMade records the grants a use case writes.
type grantsMade struct {
	identityrepository.MembershipGrants
	granted []identity.Grant
}

func (g *grantsMade) Grant(_ context.Context, grant identity.Grant) error {
	g.granted = append(g.granted, grant)
	return nil
}

// privateHarness is CreateContainer behind the real authoriser and the registry, so that the
// registry's validation and the authoriser's decision are both what the test meets.
func privateHarness(held ...identity.Membership) (*usecase.Registry, *harness, *grantsMade) {
	h := newHarness()
	authoriser := access.Service{
		Memberships: &heldRoles{held: held}, UnitOfWork: h.uow, Audit: h.audit, Clock: clock.Fixed(now),
	}
	grants := &grantsMade{}
	h.handler.Authorizer = authoriser
	h.handler.OwnHubs = authoriser
	h.handler.Grants = grants
	registry, err := usecase.NewRegistry(nil, h.handler.Descriptor())
	if err != nil {
		panic(err)
	}
	return registry, h, grants
}

// UC-ID-16 check 1: whatever their role, a person creates a private hub for themselves and owns
// it - including somebody whose only role is on one hub, and an auditor.
func TestAnyPersonCreatesAPrivateHubAndOwnsIt(t *testing.T) {
	roles := map[string]identity.Membership{
		"member":              {AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleMember},
		"contributor":         {AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleContributor},
		"viewer":              {AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleViewer},
		"guest":               {AccountID: accountID, Scope: identity.ItemScope(hubID), Role: identity.RoleGuest},
		"auditor":             {AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleAuditor},
		"administrator":       {AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleAdmin},
		"a hub's member only": {AccountID: accountID, Scope: identity.HubScope(hubID), Role: identity.RoleMember},
	}
	for name, held := range roles {
		t.Run(name, func(t *testing.T) {
			registry, h, grants := privateHarness(held)

			out, err := registry.Invoke(context.Background(), CreateContainerName, actor(),
				usecase.Input{"type": "HUB", "name": "Diary", "private": true})
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
			if out["private"] != true || len(h.containers.inserted) != 1 || !h.containers.inserted[0].Private {
				t.Fatalf("the hub was not written private: %v", out)
			}
			if len(grants.granted) != 1 {
				t.Fatalf("%d grants, want the creator's ownership", len(grants.granted))
			}
			grant := grants.granted[0]
			if grant.AccountID != accountID || grant.Role != identity.RoleOwner ||
				grant.Scope != identity.HubScope(h.containers.inserted[0].ID) {
				t.Errorf("granted %+v, want OWNER on the new hub to its creator", grant)
			}
			granted := false
			for _, entry := range h.audit.entries {
				if entry.Action == "membership.granted" {
					granted = true
				}
			}
			if !granted {
				t.Errorf("the ownership is not in the trail: %+v", h.audit.entries)
			}
		})
	}
}

// Who may not: a machine, a token without the scope, somebody who holds nothing here; and a
// collection is never private, whoever asks.
func TestAPrivateHubIsRefusedWhereItMustBe(t *testing.T) {
	member := identity.Membership{AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleMember}
	service := actor()
	service.Kind = appshared.ActorServiceAccount
	unscoped := actor()
	unscoped.Scopes = []string{"items:write"}

	cases := []struct {
		name   string
		held   []identity.Membership
		actor  appshared.ActorContext
		input  usecase.Input
		detail string
		kind   error
	}{
		{"a service account", []identity.Membership{member}, service,
			usecase.Input{"type": "HUB", "name": "Diary", "private": true}, "access.not_permitted", shared.ErrForbidden},
		{"a token without containers:write", []identity.Membership{member}, unscoped,
			usecase.Input{"type": "HUB", "name": "Diary", "private": true}, "", shared.ErrForbidden},
		{"a person holding nothing", nil, actor(),
			usecase.Input{"type": "HUB", "name": "Diary", "private": true}, "access.not_permitted", shared.ErrForbidden},
		{"a private collection", []identity.Membership{member}, actor(),
			usecase.Input{"type": "COLLECTION", "name": "Days", "parent_id": hubID.String(), "private": true},
			"containers.private_only_hubs", shared.ErrValidation},
		{"a shared hub without STRUCTURE", []identity.Membership{member}, actor(),
			usecase.Input{"type": "HUB", "name": "Family"}, "access.not_permitted", shared.ErrForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registry, h, grants := privateHarness(tc.held...)

			_, err := registry.Invoke(context.Background(), CreateContainerName, tc.actor, tc.input)
			if !errors.Is(err, tc.kind) {
				t.Fatalf("error %v, want %v", err, tc.kind)
			}
			if tc.detail != "" && shared.AsError(err).DetailCode != tc.detail {
				t.Errorf("detail %q, want %q", shared.AsError(err).DetailCode, tc.detail)
			}
			if len(h.containers.inserted) != 0 || len(grants.granted) != 0 {
				t.Error("a refused hub was written")
			}
		})
	}
}

// A shared hub is unchanged: STRUCTURE decides, and nobody is made its owner.
func TestASharedHubGrantsNobody(t *testing.T) {
	registry, h, grants := privateHarness(
		identity.Membership{AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleAdmin})

	if _, err := registry.Invoke(context.Background(), CreateContainerName, actor(),
		usecase.Input{"type": "HUB", "name": "Family"}); err != nil {
		t.Fatal(err)
	}
	if len(grants.granted) != 0 || h.containers.inserted[0].Private {
		t.Errorf("a shared hub: granted %v, private %v", grants.granted, h.containers.inserted[0].Private)
	}
}
