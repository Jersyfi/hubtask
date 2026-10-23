// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"errors"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
)

// The administrator's privacy actions are decided here, by the authoriser, and nowhere else
// (rule 2): a client that leaves a control out is a courtesy, this is the check. One refusal per
// action the people's-requests screen offers, each leaving nothing behind.

func refusingAuthorizer() *authorizerDouble {
	return &authorizerDouble{refuse: shared.ErrForbidden.WithDetail("access.not_permitted")}
}

func TestRestrictingWithoutThePermissionIsRefusedAndWritesNothing(t *testing.T) {
	for _, restricted := range []bool{true, false} {
		subjects, authorizer, sink := newSubjectStore(), refusingAuthorizer(), &auditSink{}

		_, err := newRestriction(subjects, authorizer, sink).
			Execute(context.Background(), actor(), RestrictCommand{AccountID: subjectID, Restricted: restricted})
		if !errors.Is(err, shared.ErrForbidden) {
			t.Fatalf("restricted=%v: answered %v, want the refusal", restricted, err)
		}
		if len(authorizer.requests) != 1 || authorizer.requests[0].Permission != domainservice.PermissionManageMembers {
			t.Errorf("restricted=%v: asked %v", restricted, authorizer.requests)
		}
		if len(subjects.calls) != 0 || len(sink.entries) != 0 {
			t.Errorf("restricted=%v: a refused restriction touched %v, recorded %d", restricted, subjects.calls, len(sink.entries))
		}
	}
}

func TestWithdrawingSomebodyElsesConsentWithoutThePermissionIsRefused(t *testing.T) {
	consents, authorizer, sink := &consentStore{endings: 1}, refusingAuthorizer(), &auditSink{}

	_, err := newWithdrawal(consents, authorizer, sink).
		Execute(context.Background(), actor(), WithdrawCommand{AccountID: subjectID, Purpose: "ai_processing"})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("answered %v, want the refusal", err)
	}
	if len(consents.stored) != 0 || len(consents.ended) != 0 || len(sink.entries) != 0 {
		t.Error("a refused withdrawal changed the register")
	}
}

func TestReadingTheCasesWithoutThePermissionIsRefused(t *testing.T) {
	h := newHarness()
	h.authorizer = refusingAuthorizer()

	if _, err := (ListDataSubjectRequests{Cases: h.cases()}).
		Execute(context.Background(), actor(), ListQuery{}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("answered %v, want the refusal", err)
	}
	if len(h.requests.asked) != 0 {
		t.Error("a refused reader's filter reached the store")
	}
}

// Every step a case is moved by - start, answer, refuse - asks before it writes.
func TestMovingACaseWithoutThePermissionIsRefusedAndLeavesItAsItWas(t *testing.T) {
	for _, status := range []domain.Status{domain.StatusInProgress, domain.StatusCompleted, domain.StatusRejected} {
		h := newHarness()
		h.authorizer = refusingAuthorizer()
		received := erasureCase("")
		received.Status = domain.StatusReceived
		h.requests.stored[received.ID] = received

		_, err := (UpdateDataSubjectRequest{Cases: h.cases()}).
			Execute(context.Background(), actor(), UpdateCommand{RequestID: received.ID, Status: status})
		if !errors.Is(err, shared.ErrForbidden) {
			t.Fatalf("%s: answered %v, want the refusal", status, err)
		}
		if got := h.requests.stored[received.ID].Status; got != domain.StatusReceived {
			t.Errorf("%s: the refused case moved to %s", status, got)
		}
		if len(h.audit.entries) != 0 || len(h.jobs.requests) != 0 {
			t.Errorf("%s: a refused step recorded or queued something", status)
		}
	}
}

func TestPreviewingAnErasureWithoutThePermissionIsRefused(t *testing.T) {
	rig := newPreviewRig(t)
	rig.authorizer.refuse = shared.ErrForbidden.WithDetail("access.not_permitted")
	request := erasureCase("")
	request.Status = domain.StatusReceived

	if _, err := rig.preview(request, nil); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("answered %v, want the refusal", err)
	}
}
