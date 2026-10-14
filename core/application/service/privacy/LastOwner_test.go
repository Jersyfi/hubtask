// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"testing"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ownersOf answers the workspace's owners, and whether the owner lock was taken before they were read.
type ownersOf struct {
	accounts []shared.ID
	locked   bool
	readLate bool
}

func (o *ownersOf) LockOwners(context.Context) error {
	o.locked = true
	return nil
}

func (o *ownersOf) OwnerPaths(context.Context) ([]identityrepo.OwnerPath, error) {
	if !o.locked {
		o.readLate = true
	}
	paths := make([]identityrepo.OwnerPath, 0, len(o.accounts))
	for _, account := range o.accounts {
		paths = append(paths, identityrepo.OwnerPath{AccountID: account})
	}
	return paths, nil
}

func lastOwnerRefusal(t *testing.T, err error) bool {
	t.Helper()
	problem := shared.AsError(err)
	return problem != nil && problem.Code == shared.ErrConflict.Code && problem.DetailCode == lastOwnerCode
}

// domain-model.md §3.2: an erasure of the workspace's only owner is refused with the last-owner rule's
// code, in both modes, and nothing of the person is touched; with a second owner it is carried out.
func TestAnErasureOfTheLastOwnerIsRefused(t *testing.T) {
	for _, mode := range []domain.ErasureMode{domain.ModeFullDelete, domain.ModeAnonymize} {
		harness := newErasureHarness()
		owners := &ownersOf{accounts: []shared.ID{subjectID}}
		eraser := harness.eraser()
		eraser.Owners = owners

		_, err := eraser.Erase(context.Background(), actor(), erasureCase(mode))
		if !lastOwnerRefusal(t, err) {
			t.Fatalf("%s: erasing the only owner answered %v, want memberships.last_owner", mode, err)
		}
		if owners.readLate {
			t.Errorf("%s: the owners were read without the owner lock", mode)
		}
		if harness.storage.deleted || harness.storage.anonymised {
			t.Errorf("%s: the account was erased anyway", mode)
		}

		owners.accounts = append(owners.accounts, accountID)
		if _, err := eraser.Erase(context.Background(), actor(), erasureCase(mode)); err != nil {
			t.Errorf("%s: with a second owner the erasure was refused: %v", mode, err)
		}
	}
}

// A hold that keeps the account keeps an owner: the person stays, restricted, and the workspace is
// not left without one. The remainder that erases the account later asks again.
func TestAKeptAccountIsNotHeldToTheLastOwnerRule(t *testing.T) {
	harness := newErasureHarness()
	harness.holds.holds = lifecycle.Holds{{
		ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000b9"), Scope: lifecycle.HoldTenant,
		Reason: "Litigation",
	}}
	eraser := harness.eraser()
	eraser.Owners = &ownersOf{accounts: []shared.ID{subjectID}}

	erased, err := eraser.Erase(context.Background(), actor(), erasureCase(domain.ModeFullDelete))
	if err != nil {
		t.Fatalf("an erasure a hold keeps the account of was refused: %v", err)
	}
	if !erased.AccountKept {
		t.Errorf("the account was not kept: %+v", erased)
	}
}

// Starting the erasure of the only owner is refused at once, so the person starting it learns that
// somebody else has to be made Owner first rather than finding a failed job.
func TestStartingTheErasureOfTheLastOwnerIsRefused(t *testing.T) {
	h := newHarness()
	recorded, err := (CreateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), actor(), createCommand(func(c *CreateCommand) {
			c.Kind = domain.KindErasure
		}))
	if err != nil {
		t.Fatalf("recording the case: %v", err)
	}
	cases := h.cases()
	cases.Owners = &ownersOf{accounts: []shared.ID{subjectID}}

	_, err = (UpdateDataSubjectRequest{Cases: cases}).Execute(context.Background(), actor(), UpdateCommand{
		RequestID: recorded.ID, Status: domain.StatusInProgress, ErasureMode: domain.ModeAnonymize,
	})
	if !lastOwnerRefusal(t, err) {
		t.Fatalf("starting the erasure of the only owner answered %v, want memberships.last_owner", err)
	}
	if work := queuedOf(h, "privacy.request"); len(work) != 0 {
		t.Errorf("the work was queued anyway: %+v", work)
	}
}
