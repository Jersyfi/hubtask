// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package eventbus_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/eventbus"
)

// claims is the consumption port in memory: the insert is the question, so a second claim of the
// same pair answers false.
type claims struct {
	seen map[string]bool
	err  error
	made int
}

func newClaims() *claims { return &claims{seen: map[string]bool{}} }

func (c *claims) Claim(_ context.Context, consumer string, eventID shared.ID) (bool, error) {
	c.made++
	if c.err != nil {
		return false, c.err
	}
	key := consumer + "/" + eventID.String()
	if c.seen[key] {
		return false, nil
	}
	c.seen[key] = true
	return true, nil
}

func anEvent(t *testing.T) event.Envelope {
	t.Helper()
	envelope, err := event.NewEnvelope(
		shared.MustParseID("0192f000-0000-7000-8000-0000000000e1"),
		event.ContainerCreated,
		shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		"container/0192f000-0000-7000-8000-00000000000b",
		event.Actor{Kind: shared.ActorUser},
		time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC),
		event.Cause{}, map[string]any{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}
	return envelope
}

// The at-least-once guarantee doing what it says: the work runs on the first delivery and not on
// the second, and neither is an error (test RT-4's rule, as a library function).
func TestTheWorkRunsOnceAcrossRepeatedDeliveries(t *testing.T) {
	consumed, envelope := newClaims(), anEvent(t)
	runs := 0
	work := func(context.Context) error { runs++; return nil }

	for attempt := range 3 {
		ran, err := eventbus.Once(t.Context(), consumed, "the-consumer", envelope, work)
		if err != nil {
			t.Fatalf("attempt %d failed: %v", attempt, err)
		}
		if ran != (attempt == 0) {
			t.Errorf("attempt %d reported ran = %v", attempt, ran)
		}
	}
	if runs != 1 {
		t.Errorf("the work ran %d times, want once", runs)
	}
}

// Two consumers are two claims. A dedupe keyed on the event alone would deliver an event to
// whichever subscriber asked first and to nobody else, which is the bug this key shape exists to
// make impossible.
func TestTwoConsumersEachGetTheEvent(t *testing.T) {
	consumed, envelope := newClaims(), anEvent(t)
	runs := map[string]int{}

	for _, consumer := range []string{"notifications", "webhooks"} {
		ran, err := eventbus.Once(t.Context(), consumed, consumer, envelope,
			func(context.Context) error { runs[consumer]++; return nil })
		if err != nil {
			t.Fatalf("%s failed: %v", consumer, err)
		}
		if !ran {
			t.Errorf("%s did not get the event", consumer)
		}
	}
	if runs["notifications"] != 1 || runs["webhooks"] != 1 {
		t.Errorf("runs = %v, want one each", runs)
	}
}

// The claim comes first, so a failing claim never runs the work: the record and the work commit
// together or not at all, and work done without a record is work that will be done again.
func TestAFailingClaimDoesNotRunTheWork(t *testing.T) {
	consumed, envelope := newClaims(), anEvent(t)
	consumed.err = errors.New("the database went away")

	ran, err := eventbus.Once(t.Context(), consumed, "the-consumer", envelope,
		func(context.Context) error {
			t.Error("the work ran despite a failed claim")
			return nil
		})
	if err == nil {
		t.Fatal("a failed claim reported success")
	}
	if ran {
		t.Error("a failed claim reported that the work ran")
	}
}

// A failing consumer is raised rather than swallowed. The caller's transaction then rolls back and
// takes the claim with it, which is what makes the redelivery correct - and is why there is no
// in-memory cache in front of the claim.
func TestAFailingConsumerIsRaisedSoTheClaimRollsBackWithIt(t *testing.T) {
	consumed, envelope := newClaims(), anEvent(t)
	failure := errors.New("the subscriber refused")

	ran, err := eventbus.Once(t.Context(), consumed, "the-consumer", envelope,
		func(context.Context) error { return failure })
	if !errors.Is(err, failure) {
		t.Fatalf("error = %v, want the subscriber's own", err)
	}
	if ran {
		t.Error("a failed delivery reported that the work ran")
	}

	// And on the retry - a fresh transaction, so a fresh claim - the work is offered again. The
	// double keeps its memory, which is exactly the situation a process-local cache would create
	// and the database does not: here the claim was rolled back, so the retry sees no record.
	delete(consumed.seen, "the-consumer/"+envelope.ID.String())
	ran, err = eventbus.Once(t.Context(), consumed, "the-consumer", envelope,
		func(context.Context) error { return nil })
	if err != nil || !ran {
		t.Errorf("the redelivery reported ran = %v, err = %v", ran, err)
	}
}

// The window and the events' period are one decision. Two that could drift apart would give one of
// them a value at which a consumption record outlives an event nobody can deliver again.
func TestTheWindowMatchesTheEventsOwnPeriod(t *testing.T) {
	if eventbus.RetentionWindow != 7*24*time.Hour {
		t.Errorf("the window is %v, and data-retention.md §3 gives OUTBOX_EVENT seven days",
			eventbus.RetentionWindow)
	}
}
