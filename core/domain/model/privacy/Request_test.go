// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The case a data subject request becomes (E-10, data-protection.md §4). What is under test is the
// state machine, the deadline, and the two refusals that stop a case starting work it cannot do.

var (
	now       = time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	requestID = shared.MustParseID("0192f000-0000-7000-8000-0000000000d1")
	subjectID = shared.MustParseID("0192f000-0000-7000-8000-0000000000d2")
	handlerID = shared.MustParseID("0192f000-0000-7000-8000-0000000000d3")
	targetID  = shared.MustParseID("0192f000-0000-7000-8000-0000000000d4")
)

func input(change func(*privacy.NewRequestInput)) privacy.NewRequestInput {
	in := privacy.NewRequestInput{
		ID: requestID, Kind: privacy.KindAccess, SubjectAccountID: subjectID, Now: now,
	}
	change(&in)
	return in
}

func TestACaseOpensReceivedWithTheStatutoryDeadline(t *testing.T) {
	request, err := privacy.NewRequest(input(func(*privacy.NewRequestInput) {}))
	if err != nil {
		t.Fatalf("recording the case: %v", err)
	}

	if request.Status != privacy.StatusReceived {
		t.Errorf("a new case opens %s", request.Status)
	}
	if !request.DueAt.Equal(now.Add(privacy.DefaultDeadline)) {
		t.Errorf("the deadline is %s, want thirty days from receipt", request.DueAt)
	}
	// The scope is this workspace unless somebody asks wider: crossing the tenant boundary is
	// never what a caller gets by leaving a field out.
	if request.Scope != privacy.ScopeTenant {
		t.Errorf("a case with no scope opened as %s", request.Scope)
	}
}

func TestADeadlineTheCallerNamesIsKept(t *testing.T) {
	sooner := now.Add(7 * 24 * time.Hour)

	request, err := privacy.NewRequest(input(func(in *privacy.NewRequestInput) { in.DueAt = sooner }))
	if err != nil {
		t.Fatalf("recording the case: %v", err)
	}
	if !request.DueAt.Equal(sooner) {
		t.Errorf("the deadline is %s, want the one the caller named", request.DueAt)
	}
}

func TestACaseNobodyCouldAnswerIsRefused(t *testing.T) {
	cases := map[string]func(*privacy.NewRequestInput){
		"no subject at all": func(in *privacy.NewRequestInput) {
			in.SubjectAccountID, in.SubjectEmail = "", ""
		},
		"a kind that does not exist": func(in *privacy.NewRequestInput) { in.Kind = "TELEPATHY" },
		"a scope that does not exist": func(in *privacy.NewRequestInput) {
			in.Scope = "EVERYWHERE"
		},
		"a deadline that has passed": func(in *privacy.NewRequestInput) {
			in.DueAt = now.Add(-time.Hour)
		},
	}

	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := privacy.NewRequest(input(change)); err == nil {
				t.Fatal("the case was recorded")
			}
		})
	}
}

// A request from somebody who never got as far as an account is still a request.
func TestAnAddressIsEnoughOfASubject(t *testing.T) {
	request, err := privacy.NewRequest(input(func(in *privacy.NewRequestInput) {
		in.SubjectAccountID, in.SubjectEmail = "", " anna@example.org "
	}))
	if err != nil {
		t.Fatalf("recording the case: %v", err)
	}
	if request.SubjectEmail != "anna@example.org" {
		t.Errorf("the address came back as %q", request.SubjectEmail)
	}
}

func TestTheStateMachineIsTheOneTheDocumentNames(t *testing.T) {
	allowed := map[privacy.Status][]privacy.Status{
		privacy.StatusReceived:   {privacy.StatusInProgress, privacy.StatusRejected},
		privacy.StatusInProgress: {privacy.StatusCompleted, privacy.StatusRejected},
		privacy.StatusCompleted:  {},
		privacy.StatusRejected:   {},
	}
	every := []privacy.Status{
		privacy.StatusReceived, privacy.StatusInProgress,
		privacy.StatusCompleted, privacy.StatusRejected,
	}

	for from, permitted := range allowed {
		for _, to := range every {
			want := false
			for _, one := range permitted {
				if one == to {
					want = true
				}
			}
			if got := from.CanMoveTo(to); got != want {
				t.Errorf("%s → %s: %v, want %v", from, to, got, want)
			}
		}
	}
}

// An erasure that named no mode is started as the workspace default, which P-6 settled as
// anonymisation (H-13). It used to be a refusal, and the refusal was the wrong shape: a case that
// cannot start because a field nobody filled in is empty is a statutory deadline running while an
// administrator works out which of two words to type.
func TestAnErasureWithNoModeStartsAsTheDefault(t *testing.T) {
	request, err := privacy.NewRequest(input(func(in *privacy.NewRequestInput) {
		in.Kind = privacy.KindErasure
	}))
	if err != nil {
		t.Fatalf("recording the case: %v", err)
	}

	started, err := request.Start("", "", handlerID)
	if err != nil {
		t.Fatalf("starting an erasure that named no mode: %v", err)
	}
	if started.ErasureMode != privacy.DefaultErasureMode {
		t.Errorf("the case started as %s, want the default", started.ErasureMode)
	}
	// The default is anonymisation and not full deletion, and that direction is the whole of P-6:
	// a default is the setting nobody thinks about, so it must not be the one with the wider
	// blast radius.
	if privacy.DefaultErasureMode != privacy.ModeAnonymize {
		t.Errorf("the default is %s", privacy.DefaultErasureMode)
	}
}

// A mode named on the case is what the case is carried out as. The default fills a silence; it
// never overrides an answer.
func TestAModeNamedOnTheCaseIsTheModeItStartsWith(t *testing.T) {
	request, err := privacy.NewRequest(input(func(in *privacy.NewRequestInput) {
		in.Kind = privacy.KindErasure
	}))
	if err != nil {
		t.Fatalf("recording the case: %v", err)
	}

	started, err := request.Start(privacy.ModeFullDelete, "", handlerID)
	if err != nil {
		t.Fatalf("starting: %v", err)
	}
	if started.Status != privacy.StatusInProgress || started.ErasureMode != privacy.ModeFullDelete {
		t.Errorf("the case started as %+v", started)
	}
	if started.HandledBy != handlerID {
		t.Errorf("the case is handled by %s", started.HandledBy)
	}
}

// A mode that is neither of the two is still refused. The default fills an empty field, and an
// unknown value is not an empty field.
func TestAnUnknownModeIsStillRefused(t *testing.T) {
	request, err := privacy.NewRequest(input(func(in *privacy.NewRequestInput) {
		in.Kind = privacy.KindErasure
	}))
	if err != nil {
		t.Fatalf("recording the case: %v", err)
	}

	if _, err := request.Start("PARTIALLY", "", handlerID); err == nil {
		t.Fatal("an erasure started with a mode nobody defined")
	}
}

// And an export with nowhere to write is the same refusal: a copy of somebody's data has to be put
// somewhere, and this system writes archives to targets somebody approved.
func TestAnExportCannotStartWithoutATarget(t *testing.T) {
	for _, kind := range []privacy.Kind{privacy.KindAccess, privacy.KindPortability} {
		request, err := privacy.NewRequest(input(func(in *privacy.NewRequestInput) { in.Kind = kind }))
		if err != nil {
			t.Fatalf("recording the case: %v", err)
		}

		if _, err := request.Start("", "", handlerID); err == nil {
			t.Fatalf("a %s case started with nowhere to write", kind)
		}
		started, err := request.Start("", targetID, handlerID)
		if err != nil {
			t.Fatalf("starting: %v", err)
		}
		if started.TargetID != targetID {
			t.Errorf("the target came out as %s", started.TargetID)
		}
	}
}

// A kind that needs no special path starts without either.
func TestARectificationStartsWithNothingElse(t *testing.T) {
	request, err := privacy.NewRequest(input(func(in *privacy.NewRequestInput) {
		in.Kind = privacy.KindRectification
	}))
	if err != nil {
		t.Fatalf("recording the case: %v", err)
	}
	if _, err := request.Start("", "", handlerID); err != nil {
		t.Fatalf("a rectification could not start: %v", err)
	}
}

func TestACaseIsCompletedWithWhatItProduced(t *testing.T) {
	request, _ := privacy.NewRequest(input(func(*privacy.NewRequestInput) {}))
	started, _ := request.Start("", targetID, handlerID)

	done, err := started.Complete(now.Add(time.Hour), "hubtask-dsr-2026-08-26")
	if err != nil {
		t.Fatalf("completing: %v", err)
	}
	if done.Status != privacy.StatusCompleted || done.ResultArchive == "" {
		t.Errorf("the case completed as %+v", done)
	}
	if !done.CompletedAt.Equal(now.Add(time.Hour)) {
		t.Errorf("the case completed at %s", done.CompletedAt)
	}
}

// A refusal without a reason is not an answer.
func TestARejectionNeedsItsReason(t *testing.T) {
	request, _ := privacy.NewRequest(input(func(*privacy.NewRequestInput) {}))

	if _, err := request.Reject("   ", handlerID, now); err == nil {
		t.Fatal("a case was refused with no reason")
	}

	rejected, err := request.Reject("Identity could not be established", handlerID, now)
	if err != nil {
		t.Fatalf("rejecting: %v", err)
	}
	if rejected.Status != privacy.StatusRejected || rejected.RejectionReason == "" {
		t.Errorf("the case was refused as %+v", rejected)
	}
}

// A closed case does not move again. A second request is the honest way to record a second answer.
func TestAClosedCaseDoesNotMoveAgain(t *testing.T) {
	request, _ := privacy.NewRequest(input(func(*privacy.NewRequestInput) {}))
	started, _ := request.Start("", targetID, handlerID)
	done, _ := started.Complete(now, "")

	for _, step := range []func() error{
		func() error { _, err := done.Start("", targetID, handlerID); return err },
		func() error { _, err := done.Complete(now, ""); return err },
		func() error { _, err := done.Reject("changed my mind", handlerID, now); return err },
	} {
		err := step()
		if err == nil {
			t.Fatal("a completed case moved")
		}
		var problem *shared.Error
		if !errors.As(err, &problem) || problem.DetailCode != privacy.CodeTransitionRefused {
			t.Errorf("the refusal came back as %v", err)
		}
	}
}

// The deadline is what the alert watches.
func TestACaseIsOverdueOnlyWhileItIsOpen(t *testing.T) {
	request, _ := privacy.NewRequest(input(func(*privacy.NewRequestInput) {}))
	late := request.DueAt.Add(time.Hour)

	if request.Overdue(now) {
		t.Error("a case is overdue on the day it was recorded")
	}
	if !request.Overdue(late) {
		t.Error("an open case past its deadline is not overdue")
	}

	started, _ := request.Start("", targetID, handlerID)
	done, _ := started.Complete(now, "")
	if done.Overdue(late) {
		t.Error("a case that was answered is still counted as late")
	}
}
