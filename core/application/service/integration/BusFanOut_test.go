// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/eventbus"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// One event in, one publish job out. Nothing here talks to a bus, and that is the shape being
// asserted: the port forbids a subscriber from calling the outside world inside the dispatcher's
// transaction, so the only thing this consumer may do is ask the queue.
func TestTheBusFanOutQueuesOnePublishPerEvent(t *testing.T) {
	queued := &jobs{}
	fanOut := BusFanOut{Jobs: queued, Clock: clock.Fixed(now)}

	envelope := anEvent(t, event.ItemCreated)
	if err := fanOut.Deliver(context.Background(), envelope); err != nil {
		t.Fatalf("delivering: %v", err)
	}

	if len(queued.requests) != 1 {
		t.Fatalf("%d jobs queued, want 1", len(queued.requests))
	}
	request := queued.requests[0]
	if request.Kind != queue.KindBusPublish {
		t.Errorf("the job is a %s", request.Kind)
	}
	if request.TenantID != envelope.TenantID {
		t.Errorf("the job belongs to %s, want %s", request.TenantID, envelope.TenantID)
	}
	// The identifier and not the event: the handler reads it back at each attempt, so a retry
	// publishes what the first attempt would have rather than a copy that aged in a payload column.
	if request.Payload["event_id"] != envelope.ID.String() {
		t.Errorf("the job carries %v", request.Payload)
	}
}

// A queue that refuses is the dispatcher's problem, not something to swallow: the round has to
// fail so that the event is not marked as consumed by a consumer that did nothing with it.
func TestARefusedQueueFailsTheDelivery(t *testing.T) {
	refusing := &jobs{err: errors.New("the queue said no")}
	fanOut := BusFanOut{Jobs: refusing, Clock: clock.Fixed(now)}

	if err := fanOut.Deliver(context.Background(), anEvent(t, event.ItemCreated)); err == nil {
		t.Fatal("a refused enqueue was reported as a delivery")
	}
}

// A bus is a transport rather than a subscriber. Filtering here would be this system deciding what
// somebody else's consumers may see, with a redeploy needed to change its mind.
func TestTheBusWantsEveryEvent(t *testing.T) {
	for _, eventType := range []event.Type{event.ItemCreated, event.ItemCompleted, event.ContainerCreated} {
		if !(BusFanOut{}).Wants(eventType) {
			t.Errorf("the bus does not want %s", eventType)
		}
	}
}

// The prohibition of backup-restore.md §8.4, asserted where it is decided rather than where it is
// enforced. An external bus receiving a restore's events would fire whatever its consumers do -
// rules, mails, tickets in somebody else's system - and none of that may follow from a restore.
func TestTheBusIsNotGivenReplays(t *testing.T) {
	if eventbus.WantsReplay(BusFanOut{}) {
		t.Error("the bus fan-out takes replays; a restore would reach every consumer on the stream")
	}
}

// The name is what the deduplication keys on. Renaming it would make every event this consumer has
// already published look new, which after an upgrade is one duplicate per event in the retention
// window.
func TestTheBusFanOutIsNamedStably(t *testing.T) {
	if (BusFanOut{}).Name() != "bus" {
		t.Errorf("the consumer calls itself %q", (BusFanOut{}).Name())
	}
}
