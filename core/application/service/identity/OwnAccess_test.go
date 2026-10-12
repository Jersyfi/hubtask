// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"

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
