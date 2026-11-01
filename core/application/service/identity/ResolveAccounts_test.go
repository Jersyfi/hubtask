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

	out, err := ResolveAccounts{Accounts: newAccounts(account), UnitOfWork: work}.Descriptor().Handler.Invoke(
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
	handler := ResolveAccounts{Accounts: newAccounts(), UnitOfWork: &unitOfWork{}}
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
