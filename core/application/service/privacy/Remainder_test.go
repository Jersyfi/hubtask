// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"testing"

	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// The rest of an erasure a hold kept part of (UC-PRV-03 check 12). What is under test is which
// cases are seeded and how, and what a run does with a case in each state; that the erasure itself
// serves the holds is proven where it is defined.

var (
	otherCase = shared.MustParseID("0192f000-0000-7000-8000-0000000000d2")
	otherHold = shared.MustParseID("0192f000-0000-7000-8000-0000000003a2")
)

func keptUnder(hold shared.ID) domain.Kept {
	return domain.Kept{HoldID: hold, HoldScope: lifecycle.HoldContainer, Comments: 1}
}

func TestALiftedHoldSeedsOneJobPerCaseItKeptPartOf(t *testing.T) {
	kept := &keptStore{parts: map[shared.ID][]domain.Kept{
		erasureCase("").ID: {keptUnder(erasureHoldID)},
		otherCase:          {keptUnder(otherHold)},
	}}
	jobs := &queueDouble{}

	if err := (ErasureRemainders{Kept: kept, Jobs: jobs}).Seed(context.Background(), tenantID, erasureHoldID); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if len(jobs.requests) != 1 {
		t.Fatalf("%d jobs queued, want the one case the hold kept part of", len(jobs.requests))
	}
	job := jobs.requests[0]
	if job.Kind != queue.KindPrivacyErasureRemainder || job.TenantID != tenantID ||
		job.Payload["request_id"] != erasureCase("").ID.String() {
		t.Errorf("the job is %+v", job)
	}
	if job.DedupeKey != "dsr-remainder:"+erasureCase("").ID.String()+":"+erasureHoldID.String() {
		t.Errorf("the job is deduplicated on %q", job.DedupeKey)
	}
}

// The pass seeds what nothing seeded: every part kept under a hold that is no longer in force.
func TestThePassSeedsTheRemaindersOfHoldsNoLongerInForce(t *testing.T) {
	kept := &keptStore{parts: map[shared.ID][]domain.Kept{
		erasureCase("").ID: {keptUnder(erasureHoldID)},
		otherCase:          {keptUnder(otherHold)},
	}}
	jobs := &queueDouble{}
	active := lifecycle.Holds{{ID: otherHold, Scope: lifecycle.HoldContainer}}

	if err := (ErasureRemainders{Kept: kept, Jobs: jobs}).Reconcile(context.Background(), tenantID, active); err != nil {
		t.Fatalf("reconciling: %v", err)
	}
	if len(jobs.requests) != 1 || jobs.requests[0].Payload["request_id"] != erasureCase("").ID.String() {
		t.Errorf("queued %+v, want the case whose hold is gone", jobs.requests)
	}
}

func resumer(requests *requestStore, h *erasureHarness) ResumeErasure {
	return ResumeErasure{
		Requests: requests, Kept: h.kept, Eraser: h.eraserFor(requests),
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}
}

// Once nothing keeps the rest, it goes: the account of a full deletion with it, and the case stops
// naming it. The run is recorded as the rest of the erasure, not as a second one.
func TestTheRestGoesOnceNothingKeepsIt(t *testing.T) {
	requests, h := newRequestStore(), newErasureHarness()
	request := erasureCase(domain.ModeFullDelete)
	request.Status = domain.StatusCompleted
	requests.stored[request.ID] = request
	h.kept.parts = map[shared.ID][]domain.Kept{request.ID: {keptUnder(erasureHoldID)}}

	if err := resumer(requests, h).Resume(context.Background(), PerformInput{
		RequestID: request.ID, TenantID: tenantID,
	}); err != nil {
		t.Fatalf("resuming: %v", err)
	}
	if !h.storage.deleted || h.storage.commentsGone != 2 {
		t.Errorf("the rest did not go: deleted %v, comments %d", h.storage.deleted, h.storage.commentsGone)
	}
	if requests.stored[request.ID].SubjectAccountID != "" {
		t.Error("the completed case still names an account that is gone")
	}
	if part := h.kept.parts[request.ID][0]; part.Pending() {
		t.Error("the part is still pending")
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != RemainderErasedAction {
		t.Errorf("the trail holds %+v", h.audit.entries)
	}
}

// A rule acting as the person stops the rest of a full deletion. A refusal is an answer, not a
// failure to retry: the case says why, and the job succeeds.
func TestARuleStandingInTheWayIsRecordedAndNotRetried(t *testing.T) {
	requests, h := newRequestStore(), newErasureHarness()
	h.storage.runningRules = 1
	request := erasureCase(domain.ModeFullDelete)
	request.Status = domain.StatusCompleted
	requests.stored[request.ID] = request

	if err := resumer(requests, h).Resume(context.Background(), PerformInput{
		RequestID: request.ID, TenantID: tenantID,
	}); err != nil {
		t.Fatalf("a refusal was handed back to be retried: %v", err)
	}
	if h.kept.blocked[request.ID] != domain.CodeErasureBlockedByRule {
		t.Errorf("the case records %q", h.kept.blocked[request.ID])
	}
	if h.storage.deleted {
		t.Error("the account went although a rule acts as it")
	}
}

// What there is nothing to do for: a case that is gone, one that is not an erasure, one refused,
// and one whose account went already - which only marks its parts done.
func TestARemainderWithNothingLeftToEraseIsHarmless(t *testing.T) {
	for _, c := range []struct {
		name   string
		change func(*domain.Request)
		store  bool
	}{
		{name: "the case is gone"},
		{name: "not an erasure", store: true, change: func(r *domain.Request) { r.Kind = domain.KindAccess }},
		{name: "refused", store: true, change: func(r *domain.Request) { r.Status = domain.StatusRejected }},
		{name: "the account went already", store: true, change: func(r *domain.Request) { r.SubjectAccountID = "" }},
	} {
		t.Run(c.name, func(t *testing.T) {
			requests, h := newRequestStore(), newErasureHarness()
			request := erasureCase(domain.ModeFullDelete)
			if c.change != nil {
				c.change(&request)
			}
			if c.store {
				requests.stored[request.ID] = request
			}
			h.kept.parts = map[shared.ID][]domain.Kept{request.ID: {keptUnder(erasureHoldID)}}

			if err := resumer(requests, h).Resume(context.Background(), PerformInput{
				RequestID: request.ID, TenantID: tenantID,
			}); err != nil {
				t.Fatalf("resuming: %v", err)
			}
			if len(h.storage.order) != 0 {
				t.Errorf("a storage location was touched: %v", h.storage.order)
			}
		})
	}
}
