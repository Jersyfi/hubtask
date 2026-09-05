// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func accountHandler(accounts *accountStore, work *unitOfWork) GetAccount {
	return GetAccount{Accounts: accounts, UnitOfWork: work}
}

// The whole point: a client holding an identifier from a record it was allowed to read learns the
// name behind it. Without this, a shared workspace's history says "Somebody" about everybody.
func TestAMemberReadsTheNameBehindAnIdentifier(t *testing.T) {
	account := settled(t)

	got, err := accountHandler(newAccounts(account), &unitOfWork{}).
		Execute(t.Context(), self(serviceID), account.ID)
	if err != nil {
		t.Fatalf("reading another account: %v", err)
	}

	if got.DisplayName != account.DisplayName {
		t.Errorf("display name = %q, want %q", got.DisplayName, account.DisplayName)
	}
}

// The line `data-protection.md` §9 draws, asserted where it is decided rather than only in the
// schema: the projection carries a name and a kind, and none of the four fields that are the
// account holder's own business. A mapping that leaks one of them is a mapping this test fails.
func TestTheProjectionCarriesNoEmailAndNoPreferences(t *testing.T) {
	account := settled(t)

	out := accountSummaryOutput(account)

	for _, field := range []string{"email", "locale", "time_zone", "week_start"} {
		if _, present := out[field]; present {
			t.Errorf("%q travels to another member, and §9 says the visibility is minimal", field)
		}
	}
	// Named rather than counted, so that adding a field is a decision somebody makes here rather
	// than one inherited from the wider projection beside it.
	for _, field := range []string{"id", "kind", "display_name", "status"} {
		if _, present := out[field]; !present {
			t.Errorf("%q is missing, and a reader needs it", field)
		}
	}
	if len(out) != 4 {
		t.Errorf("the projection carries %d fields, want 4", len(out))
	}
	// `status` travels for a reason worth stating: an erasure writes its marker into the row where
	// the name was, and `ANONYMIZED` beside it is what tells a reader that the blank is deliberate
	// rather than missing data.
	if out.String("status") == "" {
		t.Error("no status, so a reader cannot tell an erased account from an ordinary one")
	}
}

// A credential bounds its holder rather than granting a second identity (ADR-0005). A token minted
// without `accounts:read` may not make this read, whoever holds it.
func TestATokenWithoutTheScopeMayNotReadAnAccount(t *testing.T) {
	account := settled(t)
	actor := self(serviceID)
	actor.Scopes = []string{"items:read"}

	_, err := accountHandler(newAccounts(account), &unitOfWork{}).Execute(t.Context(), actor, account.ID)
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error = %v, want forbidden", err)
	}
}

func TestAnUnauthenticatedCallerMayNotReadAnAccount(t *testing.T) {
	account := settled(t)

	_, err := accountHandler(newAccounts(account), &unitOfWork{}).
		Execute(t.Context(), appshared.ActorContext{}, account.ID)
	if !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("error = %v, want unauthenticated", err)
	}
}

// Reachable from three channels, and only one of them has a path parameter that cannot be empty.
func TestAReadWithoutAnIdentifierIsRefused(t *testing.T) {
	_, err := accountHandler(newAccounts(settled(t)), &unitOfWork{}).
		Execute(t.Context(), self(serviceID), "")
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error = %v, want a validation failure", err)
	}
	if got := shared.AsError(err).DetailCode; got != "accounts.account_required" {
		t.Errorf("the detail code is %q", got)
	}
}

// The tenant boundary is the transaction's rather than this use case's: the scope handed to the
// unit of work is the actor's tenant and never the account's, so an identifier from elsewhere does
// not resolve rather than being refused - which is what keeps a refusal from confirming that an
// account exists (multi-tenancy.md §2). The negative test against the real database is in
// test/integration; this asserts the half that is this layer's.
func TestTheAccountReadRunsAsTheActorsTenant(t *testing.T) {
	account := settled(t)
	work := &unitOfWork{}

	actor := self(serviceID)
	actor.TenantID = otherTenant

	if _, err := accountHandler(newAccounts(account), work).
		Execute(t.Context(), actor, account.ID); err != nil {
		t.Fatalf("reading: %v", err)
	}
	if work.scopes[0].TenantID != otherTenant {
		t.Errorf("transaction tenant = %s, want %s - the read would reach a foreign row",
			work.scopes[0].TenantID, otherTenant)
	}
}

// The descriptor is what makes it reachable through REST, MCP and automation. The parity gate
// checks that it is reachable; this checks it is the right thing when reached.
func TestTheAccountDescriptorIsAReadTakingOneIdentifier(t *testing.T) {
	descriptor := GetAccount{}.Descriptor()

	if !descriptor.ReadOnly {
		t.Error("the descriptor does not declare itself read-only, so it cannot be served by a replica")
	}
	if descriptor.TokenScope != accountsRead {
		t.Errorf("token scope = %q, want %q", descriptor.TokenScope, accountsRead)
	}
	if len(descriptor.Input) != 1 {
		t.Fatalf("%d input fields, want 1", len(descriptor.Input))
	}
	field := descriptor.Input[0]
	if field.Name != "account_id" || field.Kind != usecase.KindID || !field.Required {
		t.Errorf("the input field is %+v", field)
	}
}
