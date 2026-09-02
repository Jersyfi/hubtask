// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	serviceID   = shared.ID("01936f2a-7c1e-7000-8000-0000000000c1")
	otherTenant = shared.ID("018f2a1b-0000-7000-8000-0000000000cd")
)

func ownAccountHandler(accounts *accountStore, work *unitOfWork) GetOwnAccount {
	return GetOwnAccount{Accounts: accounts, UnitOfWork: work}
}

// self is an interactive session, which carries every scope the build declares - which is why
// asking for one is a bound on a token rather than a hurdle for a person (AuthenticateToken.go).
func self(accountID shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		TenantID: tenant, AccountID: accountID, AccountName: "Anna",
		Kind: shared.ActorUser, Scopes: []string{accountsRead},
	}
}

func serviceActor(accountID shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		TenantID: tenant, AccountID: accountID, AccountName: "The nightly import",
		Kind: shared.ActorServiceAccount, Scopes: []string{accountsRead},
	}
}

// The whole point of the use case: a client that has a credential and no identifier learns both
// who it is and how the product should speak to it.
func TestTheCallerReadsTheirOwnAccountWithItsPreferences(t *testing.T) {
	account := settled(t)
	accounts := newAccounts(account)

	got, err := ownAccountHandler(accounts, &unitOfWork{}).Execute(t.Context(), self(account.ID))
	if err != nil {
		t.Fatalf("reading the own account: %v", err)
	}

	if got.ID != account.ID {
		t.Errorf("account = %s, want %s", got.ID, account.ID)
	}
	if got.Locale != "de" || got.TimeZone != "Europe/Berlin" {
		t.Errorf("preferences = %q/%q, want de/Europe/Berlin", got.Locale, got.TimeZone)
	}
}

// A read may be served by a replica (multi-tenancy.md §7). A read that opened a write transaction
// would pin every sign-in in the product to the primary, and nothing would ever say so.
func TestTheReadOpensNoWriteTransaction(t *testing.T) {
	account := settled(t)
	work := &unitOfWork{}

	if _, err := ownAccountHandler(newAccounts(account), work).Execute(t.Context(), self(account.ID)); err != nil {
		t.Fatalf("reading the own account: %v", err)
	}

	if len(work.scopes) != 1 {
		t.Fatalf("transactions opened = %d, want 1", len(work.scopes))
	}
	if want := (persistence.Scope{TenantID: tenant, ActorID: account.ID}); work.scopes[0] != want {
		t.Errorf("scope = %+v, want %+v", work.scopes[0], want)
	}
}

// A service account has an account row like anybody else, and the honest answer is that row rather
// than a refusal a caller has to special-case (the task's own question, answered out loud).
func TestAServiceAccountGetsTheSameDocument(t *testing.T) {
	machine, err := domain.NewServiceAccount(serviceID, tenant, "The nightly import")
	if err != nil {
		t.Fatalf("preparing the service account: %v", err)
	}

	got, err := ownAccountHandler(newAccounts(machine), &unitOfWork{}).
		Execute(t.Context(), serviceActor(machine.ID))
	if err != nil {
		t.Fatalf("a service account may read itself: %v", err)
	}
	if got.Kind != domain.AccountServiceAccount {
		t.Errorf("kind = %s, want %s", got.Kind, domain.AccountServiceAccount)
	}
}

// A credential is a bound on its holder rather than a second identity (ADR-0005). A token minted
// without accounts:read may not read this, whoever holds it - and the failure is a scope failure,
// not a "not found", because the row is not what is missing.
func TestATokenWithoutTheScopeMayNotRead(t *testing.T) {
	account := settled(t)
	actor := self(account.ID)
	actor.Scopes = []string{"items:read"}

	_, err := ownAccountHandler(newAccounts(account), &unitOfWork{}).Execute(t.Context(), actor)
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error = %v, want forbidden", err)
	}
}

// Nobody signed in, nothing to answer.
func TestAnUnauthenticatedCallerIsRefused(t *testing.T) {
	_, err := ownAccountHandler(newAccounts(settled(t)), &unitOfWork{}).
		Execute(t.Context(), appshared.ActorContext{})
	if !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("error = %v, want unauthenticated", err)
	}
}

// The system acts without an account - a scheduler run, a job. It has no "me", and inventing one
// would be worse than saying so.
func TestAnActorWithoutAnAccountHasNoSelfToRead(t *testing.T) {
	actor := self(adminID)
	actor.AccountID = ""

	_, err := ownAccountHandler(newAccounts(settled(t)), &unitOfWork{}).Execute(t.Context(), actor)
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error = %v, want forbidden", err)
	}
}

// The tenant boundary is the transaction's, not this use case's: an actor of another tenant reads
// through a scope that does not contain the row, and row level security answers not-found rather
// than forbidden - anything else confirms the account exists (multi-tenancy.md §2).
//
// The negative test against the real database is in test/integration; this asserts the half that
// is this layer's: the scope handed to the unit of work is the actor's tenant and never the
// account's.
func TestTheTransactionRunsAsTheActorsTenant(t *testing.T) {
	account := settled(t)
	work := &unitOfWork{}

	actor := self(account.ID)
	actor.TenantID = otherTenant

	// The store is not tenant-aware - it is a map - so the read succeeds here. What is asserted is
	// the scope, because that is the whole mechanism by which it would not.
	if _, err := ownAccountHandler(newAccounts(account), work).Execute(t.Context(), actor); err != nil {
		t.Fatalf("reading: %v", err)
	}
	if work.scopes[0].TenantID != otherTenant {
		t.Errorf("transaction tenant = %s, want %s - the read would reach a foreign row",
			work.scopes[0].TenantID, otherTenant)
	}
}

// The descriptor is what makes it reachable through REST, MCP and automation. Its shape is
// asserted here rather than only by the parity gate, because the gate checks that it is reachable
// and this checks that it is the right thing when reached.
func TestTheDescriptorIsAReadThatTakesNoInput(t *testing.T) {
	descriptor := GetOwnAccount{}.Descriptor()

	if !descriptor.ReadOnly {
		t.Error("the descriptor is not marked read-only; a read that says it writes is a read nothing may cache")
	}
	if len(descriptor.Input) != 0 {
		t.Errorf("input fields = %d, want none - the actor is the identifier", len(descriptor.Input))
	}
	if descriptor.TokenScope != accountsRead {
		t.Errorf("token scope = %q, want %q", descriptor.TokenScope, accountsRead)
	}
	if descriptor.Audit.Required {
		t.Error("an ordinary read is not an auditable event (audit.md §4); only a refused one is")
	}
}
