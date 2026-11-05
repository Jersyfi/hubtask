// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type signInDetailsDouble struct {
	details []repository.SignInDetail
	asked   []shared.ID
}

func (d *signInDetailsDouble) SignInDetails(_ context.Context, ids []shared.ID) ([]repository.SignInDetail, error) {
	d.asked = ids
	return d.details, nil
}

// The people list's address and factor are an administrator's: asked of MANAGE_MEMBERS at the
// workspace before anything is read, bounded like a name lookup, and a managed account answers no
// address rather than an empty one.
func TestSignInDetailsAreAnAdministratorsRead(t *testing.T) {
	ids := []shared.ID{
		shared.MustParseID("0192f000-0000-7000-8000-00000000f001"),
		shared.MustParseID("0192f000-0000-7000-8000-00000000f002"),
	}
	store := &signInDetailsDouble{details: []repository.SignInDetail{
		{AccountID: ids[0], Email: "lena@example.com", AuthenticatorApp: true},
		{AccountID: ids[1]},
	}}
	auth := &authorizer{}
	h := ReadSignInDetails{Details: store, Authorizer: auth, UnitOfWork: &unitOfWork{}}

	out, err := h.invoke(context.Background(), adminActor(), usecase.Input{"ids": []any{ids[0].String(), ids[1].String()}})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	rows, _ := out["data"].([]usecase.Output)
	if len(rows) != 2 {
		t.Fatalf("answered %+v", out)
	}
	if rows[0]["email"] != "lena@example.com" || rows[0]["authenticator_app"] != true {
		t.Errorf("first row %+v", rows[0])
	}
	if rows[1]["email"] != nil || rows[1]["authenticator_app"] != false {
		t.Errorf("a managed account's row %+v", rows[1])
	}
	if len(auth.requests) != 1 || auth.requests[0].Permission != "MANAGE_MEMBERS" || auth.requests[0].TokenScope != membersRead {
		t.Errorf("asked %+v", auth.requests)
	}

	store.asked = nil
	refused := ReadSignInDetails{Details: store, Authorizer: &authorizer{refuse: shared.ErrForbidden}, UnitOfWork: &unitOfWork{}}
	if _, err := refused.Execute(context.Background(), adminActor(), ids); !errors.Is(err, shared.ErrForbidden) || store.asked != nil {
		t.Errorf("a reader without the right got %v and the store was asked %v", err, store.asked)
	}
	if _, err := h.Execute(context.Background(), adminActor(), make([]shared.ID, MaxResolvedAccounts+1)); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("201 accounts: %v", err)
	}
	if _, err := h.Execute(context.Background(), adminActor(), nil); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("no account: %v", err)
	}
}
