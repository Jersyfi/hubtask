// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// The job's half (E-10): the work, and the completion that goes with it. They are one step here
// rather than two calls from the worker, because a case whose archive was written and never
// completed sits in the list of what is owed for ever.

func performerFor(requests *requestStore, h *erasureHarness) Performer {
	return Performer{
		Requests: requests,
		Eraser: Eraser{
			Requests: requests, Erasure: h.storage, Pseudonyms: h.pseudonyms,
			Removals: h.removals, Objects: h.objects, Audit: h.audit,
			UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
		},
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}
}

func startedCase(kind domain.Kind, mode domain.ErasureMode) domain.Request {
	return domain.Request{
		ID:   shared.MustParseID("0192f000-0000-7000-8000-0000000000d1"),
		Kind: kind, Status: domain.StatusInProgress, SubjectAccountID: subjectID,
		ErasureMode: mode, TargetID: targetID, DueAt: now.Add(24 * 30 * 60 * 60 * 1000000000),
	}
}

func TestAnErasureCompletesTheCaseAndForgetsTheSubject(t *testing.T) {
	requests, h := newRequestStore(), newErasureHarness()
	request := startedCase(domain.KindErasure, domain.ModeFullDelete)
	if err := requests.Insert(context.Background(), request); err != nil {
		t.Fatalf("recording: %v", err)
	}

	done, err := performerFor(requests, h).Perform(context.Background(), PerformInput{
		RequestID: request.ID, TenantID: tenantID,
	})
	if err != nil {
		t.Fatalf("performing: %v", err)
	}

	if done.Status != domain.StatusCompleted || done.CompletedAt.IsZero() {
		t.Errorf("the case came back as %+v", done)
	}
	// The subject is gone, and the case may not keep pointing at a row that no longer exists - the
	// column has been nullable since `0001_init` for exactly this.
	if !done.SubjectAccountID.IsZero() {
		t.Errorf("the case still names %s", done.SubjectAccountID)
	}
	if !h.storage.deleted {
		t.Error("the erasure did not reach the account")
	}
	if requests.stored[request.ID].Status != domain.StatusCompleted {
		t.Error("the case was not written back")
	}
}

// An anonymisation keeps the account, so the case keeps naming it: the person is still there, in
// the only sense the workspace has of them.
func TestAnAnonymisationLeavesTheCasePointingAtTheAccount(t *testing.T) {
	requests, h := newRequestStore(), newErasureHarness()
	request := startedCase(domain.KindErasure, domain.ModeAnonymize)
	if err := requests.Insert(context.Background(), request); err != nil {
		t.Fatalf("recording: %v", err)
	}

	done, err := performerFor(requests, h).Perform(context.Background(), PerformInput{
		RequestID: request.ID, TenantID: tenantID,
	})
	if err != nil {
		t.Fatalf("performing: %v", err)
	}
	if done.SubjectAccountID != subjectID {
		t.Errorf("the case names %s", done.SubjectAccountID)
	}
}

// A case somebody refused between the job being queued and it being claimed is left alone: doing
// the work now would be doing it against a decision that has already been taken.
func TestACaseThatWasAnsweredInTheMeantimeIsLeftAlone(t *testing.T) {
	requests, h := newRequestStore(), newErasureHarness()
	request := startedCase(domain.KindErasure, domain.ModeFullDelete)
	request.Status = domain.StatusRejected
	if err := requests.Insert(context.Background(), request); err != nil {
		t.Fatalf("recording: %v", err)
	}

	done, err := performerFor(requests, h).Perform(context.Background(), PerformInput{
		RequestID: request.ID, TenantID: tenantID,
	})
	if err != nil {
		t.Fatalf("performing: %v", err)
	}
	if done.Status != domain.StatusRejected {
		t.Errorf("the case came back as %s", done.Status)
	}
	if len(h.storage.order) != 0 {
		t.Errorf("a refused case was carried out anyway: %v", h.storage.order)
	}
}

// The job acts as the installation rather than as whoever asked: the person who started the case
// may have gone home, and the entry says which case the work belonged to.
func TestTheWorkIsDoneByTheInstallation(t *testing.T) {
	requests, h := newRequestStore(), newErasureHarness()
	request := startedCase(domain.KindErasure, domain.ModeAnonymize)
	if err := requests.Insert(context.Background(), request); err != nil {
		t.Fatalf("recording: %v", err)
	}

	if _, err := performerFor(requests, h).Perform(context.Background(), PerformInput{
		RequestID: request.ID, TenantID: tenantID,
	}); err != nil {
		t.Fatalf("performing: %v", err)
	}

	entry := h.audit.entries[0]
	if entry.ActorKind != "SYSTEM" {
		t.Errorf("the work was recorded as done by %s", entry.ActorKind)
	}
	if entry.TargetID != request.ID {
		t.Errorf("the entry names %s rather than the case", entry.TargetID)
	}
}

func TestAPayloadThatCannotBeReadFailsTheJob(t *testing.T) {
	for _, payload := range []map[string]any{{}, {"request_id": "not-an-id"}} {
		if _, err := RequestOf(payload, tenantID); err == nil {
			t.Errorf("the payload %v was read as a case", payload)
		}
	}

	in, err := RequestOf(map[string]any{"request_id": subjectID.String()}, tenantID)
	if err != nil {
		t.Fatalf("reading the payload: %v", err)
	}
	if in.RequestID != subjectID || in.TenantID != tenantID {
		t.Errorf("the payload came back as %+v", in)
	}
}
