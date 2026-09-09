// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// The seed: one job per workspace that owes embeddings, asked for by the write that changed a
// title or a note. Nothing here decides *which* entries owe one - that is the pass's job, and
// keeping the decision there is what makes the seed cheap enough to run on every write.

type seedQueue struct {
	requests []queue.Request
	err      error
}

func (q *seedQueue) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	if q.err != nil {
		return "", q.err
	}
	q.requests = append(q.requests, request)
	return taskID, nil
}

func seedHarness() (SeedEmbedding, *seedQueue) {
	jobs := &seedQueue{}
	return SeedEmbedding{Jobs: jobs, Delay: 30 * time.Second, Clock: clock.Fixed(now)}, jobs
}

func itemEvent(eventType event.Type) event.Envelope {
	return event.Envelope{ID: taskID, Type: eventType, TenantID: tenantID, Subject: "item/" + taskID.String()}
}

// The two events that can have changed the words an entry is indexed by. Everything else an entry
// can have happen to it - a label, an assignment, a move - leaves its text alone, and asking for a
// pass after each would be a provider call for a card somebody dragged.
func TestOnlyTheEventsThatCanChangeTextAskForAPass(t *testing.T) {
	seed, _ := seedHarness()

	for _, c := range []struct {
		eventType event.Type
		want      bool
	}{
		{event.ItemCreated, true},
		{event.ItemUpdated, true},
		{event.ItemTrashed, false},
		{event.ItemMoved, false},
		{event.ItemCompleted, false},
		{event.CommentCreated, false},
		{event.ItemLabelAdded, false},
	} {
		if got := seed.Wants(c.eventType); got != c.want {
			t.Errorf("Wants(%s) = %v, want %v", c.eventType, got, c.want)
		}
	}
}

// One pending job per workspace, whatever happened: "this workspace owes embeddings" is one fact
// however many entries changed, and a burst of edits should become one pass rather than one per
// keystroke.
func TestABurstOfEditsBecomesOneJob(t *testing.T) {
	seed, jobs := seedHarness()

	for range 3 {
		if err := seed.Deliver(t.Context(), itemEvent(event.ItemUpdated)); err != nil {
			t.Fatalf("seeding failed: %v", err)
		}
	}

	if len(jobs.requests) != 3 {
		t.Fatalf("%d jobs were asked for, want one per event", len(jobs.requests))
	}
	first := jobs.requests[0]
	for _, request := range jobs.requests[1:] {
		if request.DedupeKey != first.DedupeKey {
			t.Errorf("two edits in one workspace asked under %q and %q - the queue will keep both",
				first.DedupeKey, request.DedupeKey)
		}
	}
	if first.DedupeKey != string(queue.KindAiEmbed)+":"+tenantID.String() {
		t.Errorf("the deduplication key is %q, want one per workspace", first.DedupeKey)
	}
}

// The job names the workspace, runs after the delay, and asks for the embedding kind. The delay is
// what collapses the burst above: the key only ever collapses jobs that are still pending.
func TestTheSeededJobNamesTheWorkspaceAndWaits(t *testing.T) {
	seed, jobs := seedHarness()

	if err := seed.Deliver(t.Context(), itemEvent(event.ItemCreated)); err != nil {
		t.Fatalf("seeding failed: %v", err)
	}

	request := jobs.requests[0]
	if request.Kind != queue.KindAiEmbed {
		t.Errorf("the job is of kind %q", request.Kind)
	}
	if request.TenantID != tenantID {
		t.Errorf("the job names %s, want the workspace the event came from", request.TenantID)
	}
	if want := now.Add(30 * time.Second); !request.RunAt.Equal(want) {
		t.Errorf("the job runs at %s, want %s", request.RunAt, want)
	}
}

// An event the subscriber does not want, and one that names no workspace, are both nothing to do
// rather than a failure: a subscriber that refused would stop the dispatcher over an event that
// was never its business.
func TestAnUnwantedOrTenantlessEventSeedsNothing(t *testing.T) {
	seed, jobs := seedHarness()

	if err := seed.Deliver(t.Context(), itemEvent(event.ItemTrashed)); err != nil {
		t.Fatalf("an unwanted event failed: %v", err)
	}
	tenantless := itemEvent(event.ItemUpdated)
	tenantless.TenantID = ""
	if err := seed.Deliver(t.Context(), tenantless); err != nil {
		t.Fatalf("a tenantless event failed: %v", err)
	}

	if len(jobs.requests) != 0 {
		t.Errorf("%d jobs were asked for, want none", len(jobs.requests))
	}
}

// The name is stable across versions, for the reason every consumer name is: renaming it makes
// every event it has already seen look new.
func TestTheSubscriberIsNamedForTheDeduplication(t *testing.T) {
	seed, _ := seedHarness()
	if seed.Name() != "search.embedding" {
		t.Errorf("the subscriber is called %q", seed.Name())
	}
}
