// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// A new link is answered to whoever renews it, who could accept it and arrive with the roles the
// invitation gives - so a renewal by link is held to the ceiling (identity.md §13).
func TestARenewalByLinkIsHeldToTheCeiling(t *testing.T) {
	for _, c := range ceilingCases {
		t.Run(c.name, func(t *testing.T) {
			f := newRenewFixture(t, domain.AccountInvited, true)
			f.auth.own = c.own
			f.held[invitedID] = c.held

			out, err := f.renew(t, admin())
			if c.issued {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				if _, linked := out["invitation_link"]; !linked || len(f.minter.minted) != 1 {
					t.Error("renewed without a new link")
				}
				return
			}
			if got := detailOf(err); got != "access.role_above_own" {
				t.Fatalf("answered %q (%v), want access.role_above_own", got, err)
			}
			if len(f.minter.minted)+len(f.queued.requests)+len(f.sink.entries) != 0 {
				t.Error("something went out for a refused renewal")
			}
			asked := f.auth.requests[len(f.auth.requests)-1]
			if asked.Grants != domain.RoleOwner || asked.Action != InvitationRenewedAction || asked.TargetID != invitedID {
				t.Errorf("the ceiling's question was %+v", asked)
			}
		})
	}
}

// The mail reaches the invited address, not whoever renews it: no way in for them, no ceiling.
func TestARenewalByMailIsNotHeldToTheCeiling(t *testing.T) {
	f := newRenewFixture(t, domain.AccountInvited, false)
	f.auth.own = domain.RoleAdmin
	f.held[invitedID] = []domain.Role{domain.RoleOwner}

	if _, err := f.renew(t, admin()); err != nil {
		t.Fatalf("renewing by mail: %v", err)
	}
	if len(f.queued.requests) != 1 {
		t.Errorf("queued %v, want the mail", f.queued.requests)
	}
	for _, asked := range f.auth.requests {
		if asked.Grants != "" {
			t.Errorf("a renewal by mail asked the ceiling: %+v", asked)
		}
	}
}
