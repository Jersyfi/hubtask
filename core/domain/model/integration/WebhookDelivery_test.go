// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	deliveryID = shared.ID("01936f2a-7c1e-7000-8000-0000000000e1")
	replayID   = shared.ID("01936f2a-7c1e-7000-8000-0000000000e2")
	eventID    = shared.ID("01936f2a-7c1e-7000-8000-0000000000e3")
)

func attempt(t *testing.T, number int) WebhookDelivery {
	t.Helper()
	delivery, err := NewWebhookDelivery(deliveryID, hookTenant, hookID, eventID, number, hookNow)
	if err != nil {
		t.Fatalf("building the delivery: %v", err)
	}
	return delivery
}

func TestANewDeliveryIsPendingAndCarriesItsEvent(t *testing.T) {
	delivery := attempt(t, 1)

	if delivery.Status != DeliveryPending || delivery.Attempt != 1 {
		t.Errorf("delivery = %+v", delivery)
	}
	if delivery.EventID != eventID {
		t.Errorf("event = %s, want the one being delivered", delivery.EventID)
	}

	if _, err := NewWebhookDelivery(deliveryID, hookTenant, hookID, eventID, 0, hookNow); err == nil {
		t.Error("an attempt numbered zero was accepted")
	}
	if _, err := NewWebhookDelivery(deliveryID, hookTenant, hookID, "", 1, hookNow); err == nil {
		t.Error("a delivery of no event was accepted")
	}
}

func TestASuccessLeavesNoRetryAndNoStaleError(t *testing.T) {
	failed := attempt(t, 1).Failed(500, "webhooks.target_refused", hookNow.Add(time.Minute))
	done := failed.Succeeded(200)

	if done.Status != DeliverySucceeded || done.ResponseStatus != 200 {
		t.Errorf("delivery = %+v", done)
	}
	if done.ErrorCode != "" || !done.NextAttemptAt.IsZero() {
		t.Errorf("a success left an error or a retry behind: %+v", done)
	}
}

// The decision is in the model rather than at the caller because it is the same decision every
// caller would have to make, and one that got it wrong would either retry forever or dead-letter
// early - both of which look like the system working.
func TestTheEighthAttemptIsWhereTheAttemptsStop(t *testing.T) {
	next := hookNow.Add(time.Hour)

	for number := 1; number < MaxDeliveryAttempts; number++ {
		delivery := attempt(t, number).Failed(502, "webhooks.target_refused", next)
		if delivery.Status != DeliveryFailed {
			t.Errorf("attempt %d is %s, want a retry to follow", number, delivery.Status)
		}
		if !delivery.NextAttemptAt.Equal(next) {
			t.Errorf("attempt %d has no next attempt", number)
		}
	}

	last := attempt(t, MaxDeliveryAttempts).Failed(502, "webhooks.target_refused", next)
	if !last.IsDeadLettered() {
		t.Fatalf("attempt %d is %s, want the dead letter", MaxDeliveryAttempts, last.Status)
	}
	// Nothing is coming, and the row must not claim otherwise: a next attempt on a dead-lettered
	// delivery is a retry nobody will make.
	if !last.NextAttemptAt.IsZero() {
		t.Errorf("a dead-lettered delivery names a next attempt at %v", last.NextAttemptAt)
	}
	// The error is kept. It is what an operator reads before deciding whether to replay.
	if last.ErrorCode == "" || last.ResponseStatus != 502 {
		t.Errorf("the dead letter forgot what happened: %+v", last)
	}
}

// A replay is the same event sent again, and the two halves of that sentence are both asserted:
// the event identifier is the one it always had, and the attempt carries on.
func TestAReplayKeepsTheEventAndCarriesTheAttemptOn(t *testing.T) {
	dead := attempt(t, MaxDeliveryAttempts).Failed(502, "webhooks.target_refused", time.Time{})

	replayed, err := dead.Replayed(replayID, hookNow.Add(time.Hour))
	if err != nil {
		t.Fatalf("replaying the dead letter: %v", err)
	}

	if replayed.EventID != dead.EventID {
		t.Errorf("event = %s, want %s - a subscriber deduplicates on it", replayed.EventID, dead.EventID)
	}
	if replayed.ID == dead.ID {
		t.Error("the replay reused the delivery's identifier; it is a new attempt with its own outcome")
	}
	if replayed.Attempt != dead.Attempt+1 {
		t.Errorf("attempt = %d, want %d - the log is an account of how many times this was sent",
			replayed.Attempt, dead.Attempt+1)
	}
	if replayed.Status != DeliveryPending {
		t.Errorf("status = %s, want pending", replayed.Status)
	}
}

// Replaying something still being retried would put two attempts of one event in flight, and the
// second would arrive before the first had given up.
func TestOnlyADeadLetteredDeliveryCanBeReplayed(t *testing.T) {
	for _, delivery := range []WebhookDelivery{
		attempt(t, 1),
		attempt(t, 1).Failed(500, "webhooks.target_refused", hookNow.Add(time.Minute)),
		attempt(t, 1).Succeeded(200),
	} {
		_, err := delivery.Replayed(replayID, hookNow)
		if !errors.Is(err, shared.ErrConflict) {
			t.Errorf("a %s delivery was replayable: %v", delivery.Status, err)
		}
	}
}

// A retry and a replay are different things, and both guards are load bearing: a retry of a dead
// letter would restart a ladder that was deliberately ended, and a replay of a failed attempt
// would put two attempts of one event in flight.
func TestARetryFollowsAFailureAndAReplayFollowsADeadLetter(t *testing.T) {
	failed := attempt(t, 1).Failed(503, "webhooks.target_unavailable", hookNow.Add(time.Minute))
	dead := attempt(t, MaxDeliveryAttempts).Failed(503, "webhooks.target_unavailable", time.Time{})

	next, err := failed.Retried(replayID, hookNow)
	if err != nil {
		t.Fatalf("retrying a failed attempt: %v", err)
	}
	if next.Attempt != failed.Attempt+1 || next.EventID != failed.EventID {
		t.Errorf("the retry is %+v", next)
	}

	if _, err := dead.Retried(replayID, hookNow); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a dead letter was retried automatically: %v", err)
	}
	if _, err := failed.Replayed(replayID, hookNow); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("an attempt still being retried was replayed by hand: %v", err)
	}
	if _, err := dead.Replayed(replayID, hookNow); err != nil {
		t.Errorf("a dead letter could not be replayed: %v", err)
	}
}
