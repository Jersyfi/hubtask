// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

func machines(accounts *accountStore, auth *authorizer, sink *auditSink) ServiceAccounts {
	return ServiceAccounts{
		Accounts: accounts, Authorizer: auth, Audit: sink,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: serviceAccountID},
	}
}

// What a service account is, in one test: active from the moment it exists, with no address to
// sign in with and nothing waiting to be accepted.
func TestAServiceAccountIsActiveAndHasNoAddress(t *testing.T) {
	accounts, auth, sink := newAccounts(), &authorizer{}, &auditSink{}

	created, err := CreateServiceAccount{Accounts: machines(accounts, auth, sink)}.
		Execute(t.Context(), holder(), "the nightly export")
	if err != nil {
		t.Fatalf("creating a service account failed: %v", err)
	}

	if created.Kind != domain.AccountServiceAccount {
		t.Errorf("kind = %s, want a service account", created.Kind)
	}
	if created.Email != "" {
		t.Errorf("a service account was given the address %q", created.Email)
	}
	// No INVITED step: there is no mailbox to prove control of, so there is nothing to accept.
	if created.Status != domain.AccountActive {
		t.Errorf("status = %s, want active from the start", created.Status)
	}
	if err := created.Verify(); err != nil {
		t.Errorf("a fresh service account may not act: %v", err)
	}

	if len(accounts.inserted) != 1 || accounts.inserted[0].ID != serviceAccountID {
		t.Errorf("stored %+v", accounts.inserted)
	}
}

// It is administered by whoever answers for who has access, which is the same permission that
// governs its tokens - and the same reasoning.
func TestCreatingAndListingNeedMemberManagement(t *testing.T) {
	accounts, auth, sink := newAccounts(), &authorizer{}, &auditSink{}
	set := machines(accounts, auth, sink)

	if _, err := (CreateServiceAccount{Accounts: set}).Execute(t.Context(), holder(), "a machine"); err != nil {
		t.Fatalf("creating failed: %v", err)
	}
	if _, err := (ListServiceAccounts{Accounts: set}).Execute(t.Context(), holder()); err != nil {
		t.Fatalf("listing failed: %v", err)
	}

	if len(auth.requests) != 2 {
		t.Fatalf("the authoriser was asked %d times, want twice", len(auth.requests))
	}
	for _, request := range auth.requests {
		if request.Permission != service.PermissionManageMembers {
			t.Errorf("permission = %s, want %s", request.Permission, service.PermissionManageMembers)
		}
		if request.TokenScope != membersWrite {
			t.Errorf("token scope = %s, want %s", request.TokenScope, membersWrite)
		}
	}

	auth.refuse = shared.ErrForbidden.WithDetail("access.not_permitted")
	if _, err := (CreateServiceAccount{Accounts: set}).Execute(t.Context(), holder(), "another"); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("error = %v, want the authoriser's refusal", err)
	}
	if len(accounts.inserted) != 1 {
		t.Error("a refused creation still wrote an account")
	}
}

// A machine with no name would appear nameless beside every action it takes, and the trail's whole
// job is to be readable months later (audit.md §2).
func TestAServiceAccountNeedsAName(t *testing.T) {
	accounts, auth, sink := newAccounts(), &authorizer{}, &auditSink{}

	_, err := CreateServiceAccount{Accounts: machines(accounts, auth, sink)}.
		Execute(t.Context(), holder(), "   ")
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error = %v, want a validation error", err)
	}
	if len(sink.entries) != 0 {
		t.Error("a refused creation wrote an audit entry")
	}
}

// The entry says which way in was created, by name: unlike a token's name, which is one person's
// note about their own credential, this is the label the trail will carry from now on.
func TestTheCreationEntryNamesTheAccount(t *testing.T) {
	accounts, auth, sink := newAccounts(), &authorizer{}, &auditSink{}

	if _, err := (CreateServiceAccount{Accounts: machines(accounts, auth, sink)}).
		Execute(t.Context(), holder(), "the nightly export"); err != nil {
		t.Fatalf("creating failed: %v", err)
	}
	if len(sink.entries) != 1 {
		t.Fatalf("wrote %d entries, want one", len(sink.entries))
	}

	entry := sink.entries[0]
	if entry.Action != ServiceAccountCreatedAction || entry.TargetType != accountTarget {
		t.Errorf("entry = %s on %s", entry.Action, entry.TargetType)
	}
	if entry.TargetLabel != "the nightly export" {
		t.Errorf("target label = %q", entry.TargetLabel)
	}
}

// The catalogue path, which is what REST, MCP and a rule all reach the use cases through.
func TestTheTwoReachTheirWorkThroughTheirDescriptors(t *testing.T) {
	// One machine already there and one created by the call, so the listing has something to be
	// wrong about: a fake that answers a single row cannot tell a filter from a lucky guess.
	existing := domain.Account{
		ID: strangerID, TenantID: tenant, Kind: domain.AccountServiceAccount,
		DisplayName: "the nightly export", Status: domain.AccountActive,
	}
	person := domain.Account{
		ID: adminID, TenantID: tenant, Kind: domain.AccountUser,
		DisplayName: "Anna", Status: domain.AccountActive,
	}
	accounts, auth, sink := newAccounts(existing, person), &authorizer{}, &auditSink{}
	set := machines(accounts, auth, sink)

	create := CreateServiceAccount{Accounts: set}.Descriptor()
	list := ListServiceAccounts{Accounts: set}.Descriptor()

	input := usecase.Input{"display_name": "a second machine"}
	if err := create.ValidateInput(input); err != nil {
		t.Fatalf("the creation's own input is refused by its declaration: %v", err)
	}
	out, err := create.Handler.Invoke(t.Context(), holder(), input)
	if err != nil {
		t.Fatalf("creating through the descriptor failed: %v", err)
	}
	if out.String("kind") != string(domain.AccountServiceAccount) {
		t.Errorf("kind = %q", out.String("kind"))
	}

	listed, err := list.Handler.Invoke(t.Context(), holder(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing through the descriptor failed: %v", err)
	}
	rows, _ := listed["data"].([]usecase.Output)
	if len(rows) != 2 {
		t.Fatalf("listed %d accounts, want the two machines and not the person", len(rows))
	}
	for _, row := range rows {
		if row.String("id") == adminID.String() {
			t.Error("a person is in the list of service accounts")
		}
	}

	if !create.Audit.Required {
		t.Error("a write does not declare its audit obligation")
	}
	if !list.ReadOnly {
		t.Error("the listing is not annotated read-only")
	}
}
