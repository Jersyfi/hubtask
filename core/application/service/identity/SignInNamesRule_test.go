// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-ID-20 check 4: the card asks for "address or sign-in name" exactly where the workspace has a
// managed account, and nowhere else - an unknown host answers what a workspace without one answers
// (P-10, identity.md §3).
func TestTheCardAsksForASignInNameOnlyWhereOneExists(t *testing.T) {
	handler, _, _ := newRulesFixture()
	store := &managedStore{
		accountStore: newAccounts(), hashes: map[shared.ID]string{}, drawnAt: map[shared.ID]time.Time{},
	}
	handler.Managed = store

	without, err := handler.Execute(t.Context(), GetSignInRulesCommand{Host: "home.example", TenantSlug: "acme"})
	if err != nil || without.SignInNames {
		t.Fatalf("a workspace without a managed account answered %v, %v", without.SignInNames, err)
	}

	store.byID[managedID] = domain.Account{ID: managedID, SignInName: "lena", Status: domain.AccountActive}
	with, err := handler.Execute(t.Context(), GetSignInRulesCommand{Host: "home.example", TenantSlug: "acme"})
	if err != nil || !with.SignInNames {
		t.Fatalf("a workspace with a managed account answered %v, %v", with.SignInNames, err)
	}

	unknown, err := handler.Execute(t.Context(), GetSignInRulesCommand{Host: "home.example", TenantSlug: "nobody"})
	if err != nil || unknown.SignInNames {
		t.Errorf("an unknown host answered %v, %v - it must answer what an undecided workspace does",
			unknown.SignInNames, err)
	}
	if out := rulesOutput(with); out["sign_in_names"] != true {
		t.Errorf("the projection carries %v", out["sign_in_names"])
	}
}

// A member's summary of another person says whether mail reaches them, never the address.
func TestASummarySaysWhetherMailReachesThePerson(t *testing.T) {
	managed := accountSummaryOutput(domain.Account{ID: managedID, SignInName: "lena", DisplayName: "Lena"})
	addressed := accountSummaryOutput(domain.Account{ID: managedID, Email: "anna@example.org", DisplayName: "Anna"})
	if managed["has_address"] != false || addressed["has_address"] != true {
		t.Errorf("the summaries say %v and %v", managed["has_address"], addressed["has_address"])
	}
	if _, held := managed["sign_in_name"]; held {
		t.Error("the sign-in name travels to another member")
	}
}
