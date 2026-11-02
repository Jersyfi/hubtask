// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type accessDouble struct {
	held  bool
	asked shared.ID
}

func (a *accessDouble) HoldsAny(_ context.Context, accountID shared.ID) (bool, error) {
	a.asked = accountID
	return a.held, nil
}

// `has_access` is answered for the caller's own account, read for the caller and nobody else, and a
// build wired without the reading answers true - the shape before it was asked, which shows no
// notice. What it answers per scope is proven against PostgreSQL (test/integration).
func TestTheOwnAccountSaysWhetherItHoldsAnyAccess(t *testing.T) {
	account := settled(t)
	for _, tc := range []struct {
		name   string
		access AccessHolder
		want   bool
	}{
		{"holds nothing", &accessDouble{held: false}, false},
		{"holds a role somewhere", &accessDouble{held: true}, true},
		{"a build without the reading", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := ownAccountHandler(newAccounts(account), &unitOfWork{})
			handler.Access = tc.access
			registry, err := usecase.NewRegistry(nil, handler.Descriptor())
			if err != nil {
				t.Fatalf("registering: %v", err)
			}
			out, err := registry.Invoke(t.Context(), GetOwnAccountName, self(account.ID), usecase.Input{})
			if err != nil {
				t.Fatalf("reading: %v", err)
			}
			if out["has_access"] != tc.want {
				t.Errorf("has_access = %v, want %v", out["has_access"], tc.want)
			}
			if double, ok := tc.access.(*accessDouble); ok && double.asked != account.ID {
				t.Errorf("asked about %q, want the caller", double.asked)
			}
		})
	}
}

type portalsDouble struct {
	portals []repository.RequesterPortal
	asked   shared.ID
}

func (p *portalsDouble) PortalsOf(_ context.Context, accountID shared.ID) ([]repository.RequesterPortal, error) {
	p.asked = accountID
	return p.portals, nil
}

// A requester who holds nothing but a portal is told where it is (UC-SVC-05): the own account
// answers the caller's portals by collection and name, and none as an empty list.
func TestTheOwnAccountNamesTheRequesterPortals(t *testing.T) {
	account := settled(t)
	desk := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000c7")
	double := &portalsDouble{portals: []repository.RequesterPortal{{CollectionID: desk, Name: "Service desk"}}}
	handler := ownAccountHandler(newAccounts(account), &unitOfWork{})
	handler.Access = &accessDouble{held: false}
	handler.Portals = double
	registry, err := usecase.NewRegistry(nil, handler.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	out, err := registry.Invoke(t.Context(), GetOwnAccountName, self(account.ID), usecase.Input{})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	portals, _ := out["portals"].([]usecase.Output)
	if len(portals) != 1 || portals[0]["collection_id"] != desk.String() || portals[0]["name"] != "Service desk" {
		t.Errorf("portals = %v", out["portals"])
	}
	if double.asked != account.ID {
		t.Errorf("asked about %q, want the caller", double.asked)
	}

	handler.Portals = nil
	registry, _ = usecase.NewRegistry(nil, handler.Descriptor())
	out, _ = registry.Invoke(t.Context(), GetOwnAccountName, self(account.ID), usecase.Input{})
	if portals, ok := out["portals"].([]usecase.Output); !ok || len(portals) != 0 {
		t.Errorf("without the reading portals = %v, want an empty list", out["portals"])
	}
}
