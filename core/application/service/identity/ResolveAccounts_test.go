// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A screen naming many people reads them in one request: the ones that resolve, each once, and an
// identifier of no account in this workspace left out rather than failing the rest.
func TestManyNamesAreResolvedInOneRead(t *testing.T) {
	account := settled(t)
	missing := shared.ID("0192a7c0-0000-7000-8000-0000000000ff")
	work := &unitOfWork{}

	out, err := ResolveAccounts{Members: &authorizer{}, Accounts: newAccounts(account), UnitOfWork: work}.Descriptor().Handler.Invoke(
		t.Context(), self(serviceID), usecase.Input{"ids": []any{account.ID.String(), missing.String(), account.ID.String()}})
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	rows, _ := out["data"].([]usecase.Output)
	if len(rows) != 1 || rows[0].String("display_name") != account.DisplayName {
		t.Fatalf("resolved %v, want the one account once", rows)
	}
	if _, leaked := rows[0]["email"]; leaked {
		t.Error("the batch carries the email GetAccount withholds")
	}
}

// One request is a screen's worth of people, not a directory, and a token's scope still bounds it.
func TestResolvingIsBoundedAndScoped(t *testing.T) {
	handler := ResolveAccounts{Members: &authorizer{}, Accounts: newAccounts(), UnitOfWork: &unitOfWork{}}
	many := make([]shared.ID, MaxResolvedAccounts+1)
	for i := range many {
		many[i] = shared.ID("0192a7c0-0000-7000-8000-000000000001")
	}
	if _, err := handler.Execute(t.Context(), self(serviceID), many); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("201 identifiers answered %v, want a validation error", err)
	}
	if _, err := handler.Execute(t.Context(), self(serviceID), nil); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("no identifier answered %v, want a validation error", err)
	}
	actor := self(serviceID)
	actor.Scopes = []string{"items:read"}
	if _, err := handler.Execute(t.Context(), actor, []shared.ID{"0192a7c0-0000-7000-8000-000000000001"}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a token without accounts:read answered %v", err)
	}
}

// GetAccount's rule holds for the batch: a requester is no member and resolves no other account to
// a name - the portal shows the team as the team -, refused before anything is read; a batch of
// oneself alone is answered to everybody, and a build without the authoriser refuses the rest.
func TestARequesterResolvesNoOtherAccount(t *testing.T) {
	account := settled(t)
	refusing := &authorizer{refuse: shared.ErrForbidden.WithDetail("access.not_permitted")}
	handler := ResolveAccounts{Members: refusing, Accounts: newAccounts(account), UnitOfWork: &unitOfWork{}}

	if _, err := handler.Execute(t.Context(), self(serviceID), []shared.ID{account.ID}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("resolving another account = %v, want the refusal", err)
	}
	if len(refusing.requests) != 1 || !refusing.requests[0].AnyMember || refusing.requests[0].Action != AccountReadAction {
		t.Errorf("asked %+v, want any member", refusing.requests)
	}

	own, err := handler.Execute(t.Context(), self(account.ID), []shared.ID{account.ID})
	if err != nil || len(own) != 1 {
		t.Errorf("resolving oneself = %v, %v, want one's own account", own, err)
	}
	if len(refusing.requests) != 1 {
		t.Error("resolving oneself asked the member question")
	}

	unwired := ResolveAccounts{Accounts: newAccounts(account), UnitOfWork: &unitOfWork{}}
	if _, err := unwired.Execute(t.Context(), self(serviceID), []shared.ID{account.ID}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("without the authoriser = %v, want a refusal", err)
	}
}
