// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A legal hold stops a workspace's deletion (data-protection.md §5, multi-tenancy.md §5): the
// request is refused while any hold is in force, and a workspace already pending deletion stays
// pending until the last hold is lifted - its grace job removes nothing and comes back.

type holdsDouble struct {
	inForce lifecycle.Holds
	reads   int
}

func (h *holdsDouble) Active(context.Context) (lifecycle.Holds, error) {
	h.reads++
	return h.inForce, nil
}

func (h *holdsDouble) Contributors(context.Context, []shared.ID) (map[shared.ID][]shared.ID, error) {
	return nil, nil
}

// anItemHold is a hold the operator must not learn the scope of.
func anItemHold() lifecycle.Holds {
	return lifecycle.Holds{{
		ID: "018f2a1b-0000-7000-8000-0000000000a1", Scope: lifecycle.HoldItem,
		ScopeID: "018f2a1b-0000-7000-8000-0000000000a2", Reason: "Supplier dispute",
	}}
}

func TestADeletionRequestUnderAHoldIsRefusedBeforeTheNameAndTheProof(t *testing.T) {
	for _, status := range []domain.TenantStatus{domain.TenantActive, domain.TenantSuspended} {
		t.Run(string(status), func(t *testing.T) {
			f := newDeletionFixture(status)
			f.handler.Holds = &holdsDouble{inForce: anItemHold()}

			_, err := f.handler.Execute(t.Context(), operator(), deletionCommand())

			var domainErr *shared.Error
			if !errors.As(err, &domainErr) || domainErr.DetailCode != lifecycle.CodeLegalHold ||
				!errors.Is(err, shared.ErrConflict) {
				t.Fatalf("refused with %v, want the hold's conflict", err)
			}
			if scope := domainErr.Params["scope"]; scope != lifecycle.RefusalScopeWorkspace {
				t.Errorf("the operator was told the scope %q", scope)
			}
			if f.stepUp.consumed != 0 {
				t.Error("the refusal burned the operator's step-up")
			}
			if len(f.tenants.moved) != 0 || len(f.jobs.requests) != 0 {
				t.Errorf("a refused deletion moved the workspace (%v) or seeded %d jobs",
					f.tenants.moved, len(f.jobs.requests))
			}
		})
	}
}

// The write asks again, under the shared hold lock: a hold placed between the check before the
// step-up and the write is read there.
func TestADeletionRequestReadsTheHoldsInTheWriteToo(t *testing.T) {
	f := newDeletionFixture(domain.TenantActive)
	holds := &holdsDouble{}
	f.handler.Holds = holds

	if _, err := f.handler.Execute(t.Context(), operator(), deletionCommand()); err != nil {
		t.Fatalf("the deletion without a hold was refused: %v", err)
	}
	if holds.reads != 2 {
		t.Errorf("the holds were read %d times, want before the proof and in the write", holds.reads)
	}
}

func TestThePurgeOfAHeldWorkspaceRemovesNothingAndComesBackInADay(t *testing.T) {
	f := newHardDeleteFixture(domain.TenantPendingDeletion, now.Add(-time.Hour))
	f.handler.Holds = &holdsDouble{inForce: anItemHold()}

	outcome, err := f.handler.Execute(t.Context(), lifecycleTenant, purgeJobID)
	if err != nil {
		t.Fatalf("the held pass errored: %v", err)
	}
	if outcome.Deleted || !outcome.Held {
		t.Errorf("the outcome is %+v, want held and not deleted", outcome)
	}
	if len(f.store.deleted) != 0 || f.purge.structure || f.purge.deleteCalls != 0 ||
		len(f.journal.entries) != 0 {
		t.Error("something fell in a workspace under a hold")
	}
	if outcome.RunAgainIn != HeldPurgeRetry {
		t.Errorf("the job comes back in %v, want %v", outcome.RunAgainIn, HeldPurgeRetry)
	}
}

func TestThePurgeAfterTheLastHoldWasLiftedProceeds(t *testing.T) {
	f := newHardDeleteFixture(domain.TenantPendingDeletion, now.Add(-time.Hour))
	holds := &holdsDouble{}
	f.handler.Holds = holds

	outcome, err := f.handler.Execute(t.Context(), lifecycleTenant, purgeJobID)
	if err != nil || !outcome.Deleted {
		t.Fatalf("the pass answered (%+v, %v), want the deletion", outcome, err)
	}
	if holds.reads != 2 {
		t.Errorf("the holds were read %d times, want in the guard and in the fall", holds.reads)
	}
	if outcome.RunAgainIn != 0 {
		t.Errorf("a finished deletion asks to run again in %v", outcome.RunAgainIn)
	}
}

// A second request folded into a waiting job keeps the earlier moment (the queue's dedupe): the
// pass that runs early waits for the deadline rather than ending the only job.
func TestAPurgeThatRunsEarlyWaitsForTheDeadline(t *testing.T) {
	f := newHardDeleteFixture(domain.TenantPendingDeletion, now.Add(36*time.Hour))
	f.handler.Holds = &holdsDouble{}

	outcome, err := f.handler.Execute(t.Context(), lifecycleTenant, purgeJobID)
	if err != nil || outcome.Deleted {
		t.Fatalf("the early pass answered (%+v, %v)", outcome, err)
	}
	if outcome.RunAgainIn != 36*time.Hour {
		t.Errorf("the job comes back in %v, want at the deadline", outcome.RunAgainIn)
	}
}

// The listing says a hold stands and nothing else about it: the operator learns the deletion cannot
// proceed, not what the workspace is preserving (P-01).
func TestTheListingSaysAHoldStandsAndNothingMore(t *testing.T) {
	held := adminTenantOutput(adminrepo.TenantRecord{ID: lifecycleTenant, Slug: "acme", LegalHold: true})
	free := adminTenantOutput(adminrepo.TenantRecord{ID: operatorHome, Slug: "home"})
	if held["legal_hold"] != true || free["legal_hold"] != false {
		t.Errorf("legal_hold answered %v and %v", held["legal_hold"], free["legal_hold"])
	}
	for key := range held {
		if key != "legal_hold" && strings.Contains(key, "hold") {
			t.Errorf("the listing carries %q about a hold", key)
		}
	}
}

func TestAResumedWorkspaceEndsItsPurgeJob(t *testing.T) {
	f := newHardDeleteFixture(domain.TenantActive, now.Add(-time.Hour))
	f.handler.Holds = &holdsDouble{}

	outcome, err := f.handler.Execute(t.Context(), lifecycleTenant, purgeJobID)
	if err != nil || outcome.Deleted || outcome.RunAgainIn != 0 {
		t.Errorf("the pass answered (%+v, %v), want a quiet end", outcome, err)
	}
}
